package inventory

import "github.com/redis/go-redis/v9"

// Every script touches the keys of one hash tag only (one shard, one hold home or the event's
// meta tag), so the engine runs unchanged on Redis Cluster. Scripts that settle a shard build the
// shard's counter keys from a prefix of the same tag (ARGV), never a key of another slot.

// shardReserve takes up to ARGV[1] units of one instance on one shard.
// KEYS: counter hash, the hold's entry on this shard, the shard's expiry index.
// ARGV: need, deadline ms, hold id, instance part, now ms.
// Returns the units taken, or -1 not loaded, -2 frozen (a resize is running), -3 halted, -4 none
// free but lapsed entries are due.
var shardReserve = redis.NewScript(`
local alloc = redis.call('HGET', KEYS[1], 'alloc')
if not alloc then return -1 end
if redis.call('HGET', KEYS[1], 'halt') == '1' then return -3 end
if tonumber(redis.call('HGET', KEYS[1], 'frz') or '0') > tonumber(ARGV[5]) then return -2 end
local free = tonumber(alloc) - tonumber(redis.call('HGET', KEYS[1], 'used') or '0')
if free <= 0 then
  -- Nothing free: say so, and whether entries past their deadline wait for the sweeper (the
  -- caller lapses them at once and tries again: review spec M6).
  if #redis.call('ZRANGEBYSCORE', KEYS[3], '-inf', ARGV[5], 'LIMIT', 0, 1) > 0 then return -4 end
  return 0
end
local take = math.min(free, tonumber(ARGV[1]))
redis.call('HINCRBY', KEYS[1], 'used', take)
redis.call('HINCRBY', KEYS[2], ARGV[4], take)
redis.call('ZADD', KEYS[3], ARGV[2], ARGV[3])
return take
`)

// shardSettle commits or releases holds' entries on one shard, each exactly once.
// KEYS: the shard's expiry index, then one entry hash per hold. ARGV: mode (commit|release),
// counter key prefix, then the hold ids in KEYS order. Returns how many entries it settled.
var shardSettle = redis.NewScript(`
local n = 0
for j = 2, #KEYS do
  local f = redis.call('HGETALL', KEYS[j])
  for i = 1, #f, 2 do
    local k = ARGV[2] .. f[i]
    local q = tonumber(f[i + 1])
    if ARGV[1] == 'commit' then
      redis.call('HINCRBY', k, 'sold', q)
    else
      redis.call('HINCRBY', k, 'used', -q)
    end
  end
  if #f > 0 then n = n + 1 end
  redis.call('DEL', KEYS[j])
  redis.call('ZREM', KEYS[1], ARGV[j + 1])
end
return n
`)

// shardUnsell gives sold units back on one shard (a committed hold returned).
// KEYS: counter hashes. ARGV: quantities in KEYS order.
var shardUnsell = redis.NewScript(`
for i = 1, #KEYS do
  local q = tonumber(ARGV[i])
  redis.call('HINCRBY', KEYS[i], 'sold', -q)
  redis.call('HINCRBY', KEYS[i], 'used', -q)
end
return 1
`)

// shardFreeze stops new holds on one shard for a moment and reads its counters (resize step 1).
// KEYS: counter hash. ARGV: freeze until ms. Returns {used, sold, alloc}.
var shardFreeze = redis.NewScript(`
if not redis.call('HGET', KEYS[1], 'alloc') then
  redis.call('HSET', KEYS[1], 'alloc', 0, 'used', 0, 'sold', 0)
end
redis.call('HSET', KEYS[1], 'frz', ARGV[1])
return {tonumber(redis.call('HGET', KEYS[1], 'used') or '0'), tonumber(redis.call('HGET', KEYS[1], 'sold') or '0'), tonumber(redis.call('HGET', KEYS[1], 'alloc') or '0')}
`)

// shardAlloc sets a shard's allocation and lifts the freeze (resize step 2).
var shardAlloc = redis.NewScript(`
redis.call('HSET', KEYS[1], 'alloc', ARGV[1], 'frz', 0)
return 1
`)

// shardSnapshot reads one shard of one instance for the reconciler: alloc, used, sold, the units
// its live entries hold and whether it is halted. KEYS: counter hash, expiry index.
// ARGV: entry key prefix, instance part.
var shardSnapshot = redis.NewScript(`
local held = 0
local ids = redis.call('ZRANGE', KEYS[2], 0, -1)
for _, id in ipairs(ids) do
  held = held + tonumber(redis.call('HGET', ARGV[1] .. id, ARGV[2]) or '0')
end
local h = redis.call('HMGET', KEYS[1], 'alloc', 'used', 'sold', 'halt')
return {tonumber(h[1] or '0'), tonumber(h[2] or '0'), tonumber(h[3] or '0'), held, h[4] == '1' and 1 or 0}
`)

// shardRepair applies a reconciler correction to one shard.
// KEYS: counter hash. ARGV: field, delta, and for 'absorb' the most it may take from free stock.
// mode used: used += delta. mode alloc: alloc += delta (a negative delta only from free stock).
// mode absorb: take min(free, delta) and add it to used and sold; returns what it took.
var shardRepair = redis.NewScript(`
local mode = ARGV[1]
local d = tonumber(ARGV[2])
if mode == 'used' then
  redis.call('HINCRBY', KEYS[1], 'used', d)
  return d
end
local alloc = tonumber(redis.call('HGET', KEYS[1], 'alloc') or '0')
local used = tonumber(redis.call('HGET', KEYS[1], 'used') or '0')
local free = math.max(0, alloc - used)
if mode == 'alloc' then
  if d < 0 then d = -math.min(free, -d) end
  redis.call('HINCRBY', KEYS[1], 'alloc', d)
  return d
end
if mode == 'absorb' then
  local take = math.min(free, d)
  redis.call('HINCRBY', KEYS[1], 'used', take)
  redis.call('HINCRBY', KEYS[1], 'sold', take)
  return take
end
if mode == 'halt' then
  redis.call('HSET', KEYS[1], 'halt', ARGV[2])
  return 1
end
return 0
`)

// The hold's own record lives on its home tag: state (reserving, held, committed, released,
// returned), deadline and where its units are ("shard,instance,qty;..."). Its state decides every
// race between a payment, a release and the sweepers.

// masterBegin: KEYS hold, home expiry index. ARGV deadline ms, hold id. 0 when the hold exists.
var masterBegin = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
redis.call('HSET', KEYS[1], 'state', 'reserving', 'deadline', ARGV[1])
redis.call('ZADD', KEYS[2], ARGV[1], ARGV[2])
return 1
`)

// masterFinish: KEYS hold. ARGV draws. Returns the state after (held when it worked).
var masterFinish = redis.NewScript(`
local st = redis.call('HGET', KEYS[1], 'state')
if st ~= 'reserving' then return st or '' end
redis.call('HSET', KEYS[1], 'state', 'held', 'd', ARGV[1])
return 'held'
`)

// masterAbort drops a hold whose reserve failed part way (its entries are released by the caller).
// KEYS hold, home expiry index. ARGV hold id.
var masterAbort = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'state') == 'reserving' then redis.call('DEL', KEYS[1]) end
redis.call('ZREM', KEYS[2], ARGV[1])
return 1
`)

// masterCommit: KEYS hold, home expiry index, home committed index. ARGV hold id, now ms.
// Returns {OK|ALREADY|NOHOLD, draws}.
var masterCommit = redis.NewScript(`
local st = redis.call('HGET', KEYS[1], 'state')
if st == 'committed' then return {'ALREADY', redis.call('HGET', KEYS[1], 'd') or ''} end
if st ~= 'held' then return {'NOHOLD', ''} end
redis.call('HSET', KEYS[1], 'state', 'committed', 'cat', ARGV[2])
redis.call('EXPIRE', KEYS[1], 86400)
redis.call('ZREM', KEYS[2], ARGV[1])
redis.call('ZADD', KEYS[3], ARGV[2], ARGV[1])
return {'OK', redis.call('HGET', KEYS[1], 'd') or ''}
`)

// masterRelease: KEYS home expiry index, then holds. ARGV hold ids. For each hold returns
// "R:<draws>" released now, "C" committed, "N" not live.
var masterRelease = redis.NewScript(`
local out = {}
for j = 2, #KEYS do
  local st = redis.call('HGET', KEYS[j], 'state')
  if st == 'committed' then
    out[#out + 1] = 'C'
  elseif st == 'held' or st == 'reserving' then
    redis.call('HSET', KEYS[j], 'state', 'released')
    redis.call('EXPIRE', KEYS[j], 86400)
    redis.call('ZREM', KEYS[1], ARGV[j - 1])
    out[#out + 1] = 'R:' .. (redis.call('HGET', KEYS[j], 'd') or '')
  else
    redis.call('ZREM', KEYS[1], ARGV[j - 1])
    out[#out + 1] = 'N'
  end
end
return out
`)

// masterExtend: KEYS hold, home expiry index. ARGV deadline ms, hold id. Returns draws or false.
var masterExtend = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'state') ~= 'held' then return false end
redis.call('HSET', KEYS[1], 'deadline', ARGV[1])
redis.call('ZADD', KEYS[2], ARGV[1], ARGV[2])
return redis.call('HGET', KEYS[1], 'd') or ''
`)

// masterUnsell: KEYS hold, home committed index. ARGV hold id. Returns draws or false.
var masterUnsell = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'state') ~= 'committed' then return false end
redis.call('HSET', KEYS[1], 'state', 'returned')
redis.call('ZREM', KEYS[2], ARGV[1])
return redis.call('HGET', KEYS[1], 'd') or ''
`)

// masterRebuild writes a hold rebuilt from Postgres, unless it exists.
// KEYS hold, home expiry index. ARGV deadline ms, draws, hold id.
var masterRebuild = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
redis.call('HSET', KEYS[1], 'state', 'held', 'deadline', ARGV[1], 'd', ARGV[2])
redis.call('ZADD', KEYS[2], ARGV[1], ARGV[3])
return 1
`)

// metaRebuild loads an event's counters from Postgres onto shard 0 (its tag is the meta tag),
// unless the event is loaded. The next hold re-splits each instance over its shards.
// KEYS: ready, shards, shard-0 expiry index, then per instance: meta, counter; then per hold: entry.
// ARGV: n instances, m holds, then per instance: part, id, used, sold; then per hold: id, deadline,
// k, then k pairs (part, qty).
var metaRebuild = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
local n = tonumber(ARGV[1])
local m = tonumber(ARGV[2])
local a = 3
for i = 1, n do
  local meta = KEYS[3 + (i - 1) * 2 + 1]
  local ctr = KEYS[3 + (i - 1) * 2 + 2]
  redis.call('DEL', meta, ctr)
  redis.call('HSET', meta, 'cap', -1, 'n', 1, 'over', 0, 'inst', ARGV[a + 1])
  redis.call('HSET', ctr, 'alloc', ARGV[a + 2], 'used', ARGV[a + 2], 'sold', ARGV[a + 3])
  a = a + 4
end
for j = 1, m do
  local hk = KEYS[3 + 2 * n + j]
  local id = ARGV[a]; local deadline = ARGV[a + 1]; local k = tonumber(ARGV[a + 2])
  a = a + 3
  redis.call('DEL', hk)
  for i = 1, k do
    redis.call('HINCRBY', hk, ARGV[a], ARGV[a + 1])
    a = a + 2
  end
  redis.call('ZADD', KEYS[3], deadline, id)
end
local s = tonumber(redis.call('GET', KEYS[2]) or '0')
if s < 1 then redis.call('SET', KEYS[2], 1) end
redis.call('SET', KEYS[1], '1')
return 1
`)
