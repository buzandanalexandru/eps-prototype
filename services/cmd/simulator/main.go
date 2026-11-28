// Command simulator drives virtual fans and bots against rehearsal sales (step 6, E7, R3). It runs
// as a one-replica Deployment that asks the core's internal port for work every 2 seconds and has
// no database or Redis access: only HTTP to the gateway, the core and the providers' hosted pages,
// with a token issuer of its own that the core and gateway accept on rehearsals only.
package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simulation"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wire"
)

func main() {
	log := service.Logger("simulator")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	priv, _, err := auth.SimKeysFromEnv(os.Getenv, true)
	if err != nil {
		log.Error("simulator token key", "error", err.Error())
		os.Exit(1)
	}
	inflight, _ := strconv.Atoi(os.Getenv("SIM_MAX_INFLIGHT"))
	if inflight <= 0 || inflight > 2000 {
		inflight = 200
	}
	trim := func(k, def string) string { return strings.TrimRight(service.Env(k, def), "/") }
	e := &simulation.Engine{
		Endpoints: simulation.Endpoints{
			Gateway:  trim("GATEWAY_URL", "http://localhost:8081"),
			Core:     trim("CORE_URL", "http://localhost:8080"),
			Internal: trim("CORE_SIM_URL", "http://localhost:8089"),
			PSP:      map[string]string{"a": trim("PSP_A_URL", "http://localhost:8091"), "b": trim("PSP_B_URL", "http://localhost:8092")},
			Return:   service.Env("SIM_RETURN_URL", "http://localhost:3000/sim-return"),
		},
		Token:       wire.SimControlToken(log),
		Minter:      &simulation.Minter{Key: priv, Issuer: auth.SimIssuerForEnv(service.Env("EPS_ENV", "dev"))},
		MaxInflight: inflight,
		Log:         log,
	}
	go e.Run(ctx)
	log.Info("simulator ready", "maxInflight", inflight)
	service.Serve(log, service.SimpleHandler("simulator", log, nil, nil, nil))
}
