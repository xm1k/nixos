--!nonstrict
-- Owns the snow level (0..10, persisted) and the idle watch.
-- IPC: noctalia msg plugin xm1k/snow:ctl all <level N | front | back>

local DEFAULT_LEVEL = 5
local path = (noctalia.pluginDataDir() or "/tmp") .. "/level"

local function clampLevel(n)
  return math.clamp(math.floor(n + 0.5), 0, 10)
end

local level = DEFAULT_LEVEL
local raw = noctalia.readFile(path)
if raw and tonumber(raw) then
  level = clampLevel(tonumber(raw))
end
noctalia.state.set("level", level)
noctalia.state.set("front", false)

noctalia.idle.watch(noctalia.getConfig("idle_seconds") or 120, function(idle)
  noctalia.state.set("front", idle)
end)

function onIpc(event, payload)
  if event == "level" then
    local n = tonumber(payload)
    if n == nil then
      noctalia.log("snow: bad level '" .. tostring(payload) .. "'")
      return
    end
    level = clampLevel(n)
    noctalia.state.set("level", level)
    noctalia.writeFile(path, tostring(level))
  elseif event == "front" then -- preview the idle effect
    noctalia.state.set("front", true)
  elseif event == "back" then
    noctalia.state.set("front", false)
  end
end
