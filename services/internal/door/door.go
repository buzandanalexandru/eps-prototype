// Package door serves the organiser's door devices (step 7; G2, O5): access for the event's own
// door staff, device registration, the sync package (manifest pages and the event's door feed
// since a cursor, with the server time) and the idempotent upload of scans, where the first entry
// by device time wins and a later one is a recorded conflict (design D6 to D8).
package door

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

// Roles that may run a door (A3): owner, admin and box office.
var DoorRoles = []string{"owner", "admin", "box_office"}

// Limits of one call.
const (
	ManifestPage = 2000
	FeedPage     = 1000
	MaxScans     = 200
)

// Service is the door side of the core.
type Service struct {
	Pool  *pgxpool.Pool
	Snaps checkout.SnapshotSource
	Now   func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

var (
	syncC      = metrics.NewCounter("eps_door_syncs_total", "Door sync calls served.")
	scansC     = metrics.NewCounter("eps_door_scans_total", "Scans uploaded by door devices (new ones).")
	conflictsC = metrics.NewCounter("eps_door_conflicts_total", "Duplicate entries recorded as conflicts.")
)

// Staff is the caller as the door sees it.
type Staff struct {
	Org  string
	Role string
	Sub  string
}

func notFound() *apperr.Problem {
	return apperr.New(http.StatusNotFound, "EPS-EVENT-NOT-FOUND", "Event not found.", false)
}

// Access checks that the caller is door staff of the event's organiser. Another organiser's staff,
// or a role without door rights, get "not found" and no event data.
func (s *Service) Access(ctx context.Context, st Staff, eventID string) error {
	if st.Org == "" || !slices.Contains(DoorRoles, st.Role) {
		return notFound()
	}
	if _, err := uuid.Parse(eventID); err != nil {
		return notFound()
	}
	if snap, err := s.Snaps.Load(ctx, eventID); err == nil && snap != nil {
		if snap.OrganiserID == st.Org {
			return nil
		}
		return notFound()
	}
	// Off sale: the event's tickets (under the caller's own scope) still prove it is theirs.
	var n int
	if err := db.WithTx(ctx, s.Pool, db.RoleCore, st.Org, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from (select 1 from sales.tickets where event_id = $1 limit 1) x", eventID).Scan(&n)
	}); err != nil {
		return err
	}
	if n == 0 {
		return notFound()
	}
	return nil
}

// Device is a registered door device.
type Device struct {
	ID      string `json:"deviceId"`
	EventID string `json:"eventId"`
	Gate    string `json:"gate"`
}

// Register records a door device for an event at a gate.
func (s *Service) Register(ctx context.Context, st Staff, eventID, gate string) (*Device, error) {
	if err := s.Access(ctx, st, eventID); err != nil {
		return nil, err
	}
	gate = strings.Join(strings.Fields(gate), " ")
	if gate == "" || utf8.RuneCountInString(gate) > 40 {
		return nil, apperr.New(http.StatusUnprocessableEntity, "invalid_request", "Name the gate (at most 40 characters).", false)
	}
	d := &Device{ID: uuid.NewString(), EventID: eventID, Gate: gate}
	err := db.WithTx(ctx, s.Pool, db.RoleCore, st.Org, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "insert into sales.door_devices (id, organiser_id, event_id, gate, created_by) values ($1, $2, $3, $4, $5)", d.ID, st.Org, eventID, gate, st.Sub)
		return err
	})
	return d, err
}

// ManifestItem is one ticket of the event as the door knows it (no personal data except the name
// the holder gave for a named ticket).
type ManifestItem struct {
	TicketID   string     `json:"t"`
	ProductID  string     `json:"p"`
	Generation int        `json:"g"`
	State      string     `json:"s"`
	Name       *string    `json:"n,omitempty"`
	Day        *string    `json:"d,omitempty"`
	EntryAt    *time.Time `json:"ea,omitempty"`
	EntryGate  *string    `json:"eg,omitempty"`
}

// FeedRow is one row of the event's door feed.
type FeedRow struct {
	Seq        int64          `json:"seq"`
	Kind       string         `json:"kind"`
	TicketID   string         `json:"ticketId"`
	Generation *int           `json:"generation,omitempty"`
	Data       map[string]any `json:"data"`
	At         time.Time      `json:"at"`
}

// Sync is the answer of a door sync.
type Sync struct {
	ServerTime   time.Time                `json:"serverTime"`
	EventID      string                   `json:"eventId"`
	Cursor       int64                    `json:"cursor"`
	More         bool                     `json:"more"`
	Feed         []FeedRow                `json:"feed"`
	Manifest     []ManifestItem           `json:"manifest,omitempty"`
	ManifestNext string                   `json:"manifestNext,omitempty"`
	Products     map[string]snapshot.L10n `json:"products,omitempty"`
	Event        *EventInfo               `json:"event,omitempty"`
}

// EventInfo is what the door page shows about the event.
type EventInfo struct {
	Name     snapshot.L10n `json:"name"`
	StartsAt string        `json:"startsAt"`
	EndsAt   string        `json:"endsAt"`
	Timezone string        `json:"timezone"`
}

// SyncRequest is a door sync: the feed after `since`; with manifest set, a manifest page after the
// ticket id `manifestAfter` ("" for the first) and the current cursor to continue from.
type SyncRequest struct {
	Since         int64
	Manifest      bool
	ManifestAfter string
	DeviceID      string
}

// Sync answers a door device: the event's feed rows after its cursor in order (at most FeedPage,
// `more` when there are others) and, while it prepares, the manifest a page at a time.
func (s *Service) Sync(ctx context.Context, st Staff, eventID string, r SyncRequest) (*Sync, error) {
	if err := s.Access(ctx, st, eventID); err != nil {
		return nil, err
	}
	out := &Sync{EventID: eventID, Feed: []FeedRow{}}
	err := db.WithTx(ctx, s.Pool, db.RoleCore, st.Org, func(tx pgx.Tx) error {
		if r.DeviceID != "" {
			if _, err := uuid.Parse(r.DeviceID); err == nil {
				if _, err := tx.Exec(ctx, "update sales.door_devices set last_sync_at = now() where id = $1 and event_id = $2", r.DeviceID, eventID); err != nil {
					return err
				}
			}
		}
		if r.Manifest {
			// The cursor is read before the page, so a change made while the device pages through
			// the manifest is in its feed afterwards (applying it twice is harmless).
			if err := tx.QueryRow(ctx, "select coalesce((select seq from sales.door_counters where event_id = $1), 0)", eventID).Scan(&out.Cursor); err != nil {
				return err
			}
			after := r.ManifestAfter
			if _, err := uuid.Parse(after); err != nil {
				after = "00000000-0000-0000-0000-000000000000"
			}
			rows, err := tx.Query(ctx, `select t.id::text, t.product_id::text, t.generation, t.state, t.holder_name, to_char(t.picked_day, 'YYYY-MM-DD'), a.scanned_at, a.gate
				from sales.tickets t left join sales.ticket_scans a on a.organiser_id = t.organiser_id and a.ticket_id = t.id and a.result = 'admitted'
				where t.event_id = $1 and t.id > $2 order by t.id limit $3`, eventID, after, ManifestPage+1)
			if err != nil {
				return err
			}
			for rows.Next() {
				var m ManifestItem
				if err := rows.Scan(&m.TicketID, &m.ProductID, &m.Generation, &m.State, &m.Name, &m.Day, &m.EntryAt, &m.EntryGate); err != nil {
					rows.Close()
					return err
				}
				out.Manifest = append(out.Manifest, m)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			if len(out.Manifest) > ManifestPage {
				out.Manifest = out.Manifest[:ManifestPage]
				out.ManifestNext = out.Manifest[ManifestPage-1].TicketID
			}
			if out.Manifest == nil {
				out.Manifest = []ManifestItem{}
			}
			return nil
		}
		rows, err := tx.Query(ctx, `select seq, kind, ticket_id::text, generation, data, at from sales.door_feed where event_id = $1 and seq > $2 order by seq limit $3`,
			eventID, max(r.Since, 0), FeedPage+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var f FeedRow
			var raw []byte
			if err := rows.Scan(&f.Seq, &f.Kind, &f.TicketID, &f.Generation, &raw, &f.At); err != nil {
				return err
			}
			_ = json.Unmarshal(raw, &f.Data)
			out.Feed = append(out.Feed, f)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out.Feed) > FeedPage {
			out.Feed, out.More = out.Feed[:FeedPage], true
		}
		out.Cursor = max(r.Since, 0)
		if n := len(out.Feed); n > 0 {
			out.Cursor = out.Feed[n-1].Seq
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if r.Manifest && r.ManifestAfter == "" {
		if snap, err := s.Snaps.Load(ctx, eventID); err == nil && snap != nil {
			out.Event = &EventInfo{Name: snap.Event.Name, StartsAt: snap.Event.StartsAt, EndsAt: snap.Event.EndsAt, Timezone: snap.Timezone}
			out.Products = map[string]snapshot.L10n{}
			for _, p := range snap.Products {
				out.Products[p.ProductID] = p.Name
			}
		}
	}
	syncC.Inc()
	out.ServerTime = s.now().UTC()
	return out, nil
}

// Scan is one scan a device admitted locally.
type Scan struct {
	ID         string    `json:"id"`
	TicketID   string    `json:"ticketId"`
	Generation int       `json:"generation"`
	ScannedAt  time.Time `json:"scannedAt"`
}

// ScanResult is the server's decision on one uploaded scan.
type ScanResult struct {
	ID     string `json:"id"`
	Result string `json:"result"` // admitted | conflict | unknown
	// First names the entry that counts when the result is a conflict.
	FirstAt   *time.Time `json:"firstAt,omitempty"`
	FirstGate *string    `json:"firstGate,omitempty"`
}

// Upload records a device's scans, idempotently by scan id: a scan id seen before returns its
// stored result. For each ticket the first entry by device time counts; a later one is recorded as
// a conflict, and an earlier one arriving late takes the first place, the other becoming the
// conflict. Tickets are locked in id order, so two devices' uploads never deadlock.
func (s *Service) Upload(ctx context.Context, st Staff, eventID, deviceID string, scans []Scan) ([]ScanResult, error) {
	if err := s.Access(ctx, st, eventID); err != nil {
		return nil, err
	}
	if len(scans) > MaxScans {
		return nil, apperr.New(http.StatusRequestEntityTooLarge, "invalid_request", "At most 200 scans per upload.", false)
	}
	if _, err := uuid.Parse(deviceID); err != nil {
		return nil, apperr.New(http.StatusUnprocessableEntity, "invalid_request", "Unknown device.", false)
	}
	now := s.now()
	sorted := append([]Scan(nil), scans...)
	slices.SortFunc(sorted, func(a, b Scan) int { return strings.Compare(a.TicketID+a.ID, b.TicketID+b.ID) })
	results := map[string]ScanResult{}
	err := db.WithTx(ctx, s.Pool, db.RoleCore, st.Org, func(tx pgx.Tx) error {
		var gate string
		if err := tx.QueryRow(ctx, "select gate from sales.door_devices where id = $1 and event_id = $2", deviceID, eventID).Scan(&gate); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(http.StatusUnprocessableEntity, "invalid_request", "Unknown device.", false)
			}
			return err
		}
		for _, sc := range sorted {
			if _, err := uuid.Parse(sc.ID); err != nil {
				continue
			}
			if _, err := uuid.Parse(sc.TicketID); err != nil {
				results[sc.ID] = ScanResult{ID: sc.ID, Result: "unknown"}
				continue
			}
			res, err := s.record(ctx, tx, st.Org, eventID, deviceID, gate, sc, now)
			if err != nil {
				return err
			}
			results[sc.ID] = res
		}
		_, err := tx.Exec(ctx, "update sales.door_devices set last_sync_at = now() where id = $1", deviceID)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]ScanResult, 0, len(scans))
	for _, sc := range scans {
		if r, ok := results[sc.ID]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, org, eventID, deviceID, gate string, sc Scan, now time.Time) (ScanResult, error) {
	// The ticket, locked first: entries of one ticket (and repeats of one upload) are decided one
	// at a time, and what an earlier upload stored is visible once the lock is ours.
	var gen int
	if err := tx.QueryRow(ctx, "select generation from sales.tickets where id = $1 and event_id = $2 for update", sc.TicketID, eventID).Scan(&gen); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScanResult{ID: sc.ID, Result: "unknown"}, nil
		}
		return ScanResult{}, err
	}
	// A repeat of an uploaded scan: its stored result.
	var prev string
	err := tx.QueryRow(ctx, "select result from sales.ticket_scans where id = $1", sc.ID).Scan(&prev)
	if err == nil {
		return s.result(ctx, tx, sc.ID, sc.TicketID, prev)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ScanResult{}, err
	}
	at := sc.ScannedAt.UTC()
	if at.IsZero() || at.After(now.Add(5*time.Minute)) || at.Before(now.Add(-14*24*time.Hour)) {
		at = now.UTC() // a device clock far off: the arrival time stands in
	}
	g := sc.Generation
	if g < 1 || g > 65535 {
		g = gen
	}
	var firstID, firstGate, firstDevice string
	var firstAt time.Time
	err = tx.QueryRow(ctx, "select id::text, scanned_at, gate, device_id::text from sales.ticket_scans where ticket_id = $1 and result = 'admitted'", sc.TicketID).Scan(&firstID, &firstAt, &firstGate, &firstDevice)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if err := insertScan(ctx, tx, org, eventID, deviceID, gate, sc, g, at, "admitted"); err != nil {
			return ScanResult{}, err
		}
		scansC.Inc()
		if _, err := tickets.FeedTx(ctx, tx, org, eventID, "entry", sc.TicketID, &g, map[string]any{"at": at, "gate": gate, "device": deviceID, "scan": sc.ID}); err != nil {
			return ScanResult{}, err
		}
		return ScanResult{ID: sc.ID, Result: "admitted"}, nil
	case err != nil:
		return ScanResult{}, err
	}
	scansC.Inc()
	conflictsC.Inc()
	if at.Before(firstAt) {
		// This entry was first by device time: it takes the first place, the other becomes the conflict.
		if _, err := tx.Exec(ctx, "update sales.ticket_scans set result = 'conflict' where id = $1", firstID); err != nil {
			return ScanResult{}, err
		}
		if err := insertScan(ctx, tx, org, eventID, deviceID, gate, sc, g, at, "admitted"); err != nil {
			return ScanResult{}, err
		}
		if _, err := tickets.FeedTx(ctx, tx, org, eventID, "entry", sc.TicketID, &g, map[string]any{"at": at, "gate": gate, "device": deviceID, "scan": sc.ID}); err != nil {
			return ScanResult{}, err
		}
		if _, err := tickets.FeedTx(ctx, tx, org, eventID, "conflict", sc.TicketID, &g, map[string]any{
			"first": map[string]any{"at": at, "gate": gate, "device": deviceID, "scan": sc.ID},
			"dup":   map[string]any{"at": firstAt, "gate": firstGate, "device": firstDevice, "scan": firstID}}); err != nil {
			return ScanResult{}, err
		}
		return ScanResult{ID: sc.ID, Result: "admitted"}, nil
	}
	if err := insertScan(ctx, tx, org, eventID, deviceID, gate, sc, g, at, "conflict"); err != nil {
		return ScanResult{}, err
	}
	if _, err := tickets.FeedTx(ctx, tx, org, eventID, "conflict", sc.TicketID, &g, map[string]any{
		"first": map[string]any{"at": firstAt, "gate": firstGate, "device": firstDevice, "scan": firstID},
		"dup":   map[string]any{"at": at, "gate": gate, "device": deviceID, "scan": sc.ID}}); err != nil {
		return ScanResult{}, err
	}
	return ScanResult{ID: sc.ID, Result: "conflict", FirstAt: &firstAt, FirstGate: &firstGate}, nil
}

func insertScan(ctx context.Context, tx pgx.Tx, org, eventID, deviceID, gate string, sc Scan, gen int, at time.Time, result string) error {
	_, err := tx.Exec(ctx, `insert into sales.ticket_scans (organiser_id, id, event_id, ticket_id, generation, device_id, gate, scanned_at, result)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, org, sc.ID, eventID, sc.TicketID, gen, deviceID, gate, at, result)
	return err
}

// result is the current decision on a stored scan (its first entry when it is a conflict).
func (s *Service) result(ctx context.Context, tx pgx.Tx, scanID, ticketID, res string) (ScanResult, error) {
	out := ScanResult{ID: scanID, Result: res}
	if res == "conflict" {
		var at time.Time
		var gate string
		if err := tx.QueryRow(ctx, "select scanned_at, gate from sales.ticket_scans where ticket_id = $1 and result = 'admitted'", ticketID).Scan(&at, &gate); err == nil {
			out.FirstAt, out.FirstGate = &at, &gate
		}
	}
	return out, nil
}
