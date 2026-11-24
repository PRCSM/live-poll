-- Casts one vote atomically: status check, dedupe, count update, version bump,
-- durable queue entry and broadcast all happen in a single Redis step, so two
-- concurrent requests from the same voter can never both count.
--
-- KEYS: 1 meta, 2 counts, 3 voters, 4 ver, 5 stream
-- ARGV: 1 voterKey, 2 nowMs, 3 channel, 4 pollId, 5 userId, 6 ipHash, 7.. optionIds
if redis.call('EXISTS', KEYS[1]) == 0 then
  return {'not_loaded'}
end

local meta = redis.call('HMGET', KEYS[1], 'status', 'closesAt')
if meta[1] ~= 'open' or tonumber(ARGV[2]) >= tonumber(meta[2]) then
  return {'closed'}
end

if redis.call('SADD', KEYS[3], ARGV[1]) == 0 then
  return {'already_voted'}
end

local chosen = {}
for i = 7, #ARGV do
  redis.call('HINCRBY', KEYS[2], ARGV[i], 1)
  chosen[#chosen + 1] = ARGV[i]
end
redis.call('HINCRBY', KEYS[2], '_total', 1)
local ver = redis.call('INCR', KEYS[4])

redis.call('XADD', KEYS[5], 'MAXLEN', '~', 100000, '*',
  'pollId', ARGV[4], 'voterKey', ARGV[1], 'optionIds', table.concat(chosen, ','),
  'userId', ARGV[5], 'ipHash', ARGV[6], 'at', ARGV[2])

local flat = redis.call('HGETALL', KEYS[2])
local counts = {}
local total = 0
for i = 1, #flat, 2 do
  if flat[i] == '_total' then
    total = tonumber(flat[i + 1])
  else
    counts[flat[i]] = tonumber(flat[i + 1])
  end
end

local payload = cjson.encode({type = 'counts', ver = ver, counts = counts, total = total})
redis.call('PUBLISH', ARGV[3], payload)
return {'ok', payload}
