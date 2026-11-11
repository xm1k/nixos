--!nonstrict
-- Falling snow on a click-through layer surface (one runtime per output).
-- Flakes are ui.box nodes stacked in a column; each is moved to its real
-- position with translateX/translateY (paint-only, no relayout).
--
-- Level 0..10 (state "level") sets how many flakes are active and how fast /
-- windy it is; changes ramp smoothly: extra flakes trickle in from the top,
-- surplus ones finish falling and are not respawned.
-- State "front" (idle) fades the snow out, moves the surface to the overlay
-- layer (in front of windows) and fades it back in - and the reverse.

local MAX     = noctalia.getConfig("max_flakes") or 150
local SPEED   = noctalia.getConfig("speed") or 1.0
local OPACITY = noctalia.getConfig("opacity") or 0.8

local FADE_OUT = 1.2 -- s
local FADE_IN  = 2.5 -- s

local W, H = 1920, 1080
local function readSize()
  local name = layer.outputName()
  for _, o in ipairs(noctalia.outputs()) do
    if o.name == name then
      W, H = o.width, o.height
      return
    end
  end
end
readSize()

-- level-derived targets and their smoothed current values
local level = noctalia.state.get("level") or 5
local target = { count = 0, speed = 1, wind = 0 }
local cur = { speed = 1, wind = 0 }
local function applyLevel()
  local k = level / 10
  target.count = math.floor(MAX * k ^ 1.3 + 0.5) -- slow start, dense top end
  target.speed = (0.7 + 0.8 * k) * SPEED -- 0.7x .. 1.5x
  target.wind  = (6 + 34 * k * k) * SPEED -- px/s of gusting drift
end
applyLevel()
cur.speed, cur.wind = target.speed, target.wind

-- front/back placement with a fade between
local wantFront = noctalia.state.get("front") == true
local isFront = noctalia.state.get("placed") == true
local fade = 0 -- global alpha multiplier, fades in on start

local flakes = {}
local t = 0
local ticking = false

local function spawn(f, onScreen)
  local depth = math.random()
  f.depth = depth
  f.size  = 2 + math.floor(depth * 4 + 0.5)    -- 2..6 px
  f.fall  = 25 + depth * 55                    -- px/s at speed 1
  f.amp   = 8 + math.random() * 22             -- sway amplitude
  f.freq  = 0.4 + math.random() * 0.8          -- sway rad/s
  f.phase = math.random() * math.pi * 2
  f.dir   = math.random() * 0.6 + 0.7          -- per-flake wind sensitivity
  f.alpha = OPACITY * (0.45 + depth * 0.55)
  f.x     = math.random() * W
  f.y     = onScreen and math.random() * H or -f.size - math.random() * H * 0.6
  f.live  = true
end

for i = 1, MAX do
  local f = {}
  if i <= target.count then
    spawn(f, true)
  else
    f.live = false
  end
  flakes[i] = f
end

local function anyLive()
  for i = 1, MAX do
    if flakes[i].live then return true end
  end
  return false
end

local function render()
  local nodes = {}
  local stackY = 0 -- layout y of the next box inside the column
  if fade > 0.001 then
    for i = 1, MAX do
      local f = flakes[i]
      if f.live and f.y > -f.size then
        local px = f.x + math.sin(t * f.freq + f.phase) * f.amp
        table.insert(nodes, ui.box({
          key        = tostring(i),
          width      = f.size,
          height     = f.size,
          radius     = f.size / 2,
          softness   = 1,
          fill       = "#ffffff",
          opacity    = f.alpha * fade,
          translateX = px,
          translateY = f.y - stackY,
        }))
        stackY += f.size
      end
    end
  end
  layer.render(ui.column({ flexGrow = 1, align = "start", gap = 0 }, nodes))
end

local function setTicking(on)
  if on ~= ticking then
    ticking = on
    layer.setNeedsFrameTick(on)
  end
end

local function approach(v, goal, rate, dt)
  return v + (goal - v) * math.min(1, rate * dt)
end

function onFrameTick(deltaMs)
  local dt = math.min((tonumber(deltaMs) or 16) / 1000, 0.1)
  t += dt

  -- placement: fade out -> switch layer -> fade in
  if wantFront ~= isFront then
    fade = math.max(0, fade - dt / FADE_OUT)
    if fade <= 0 then
      isFront = wantFront
      noctalia.state.set("placed", isFront)
      noctalia.layer.configure("snow", { layer = isFront and "overlay" or "bottom" })
    end
  else
    fade = math.min(1, fade + dt / FADE_IN)
  end

  cur.speed = approach(cur.speed, target.speed, 0.8, dt)
  cur.wind  = approach(cur.wind, target.wind, 0.8, dt)
  local gust = cur.wind * (0.6 + 0.4 * math.sin(t * 0.23)) -- slow gusts

  for i = 1, MAX do
    local f = flakes[i]
    if f.live and i > target.count and f.y < -f.size then
      f.live = false -- level dropped before this flake even appeared
    elseif f.live then
      f.y += f.fall * cur.speed * dt
      f.x += gust * f.dir * dt
      if f.y > H + f.size then
        if i <= target.count then
          spawn(f, false)
        else
          f.live = false
        end
      elseif f.x > W + 40 then
        f.x -= W + 80
      end
    elseif i <= target.count then
      spawn(f, false)
    end
  end

  render()

  -- nothing left to animate at level 0
  if target.count == 0 and wantFront == isFront and not anyLive() then
    setTicking(false)
  end
end

noctalia.state.watch("level", function(v)
  level = tonumber(v) or level
  applyLevel()
  setTicking(true)
end)

noctalia.state.watch("front", function(v)
  wantFront = v == true
  setTicking(true)
end)

function update()
  readSize() -- pick up resolution changes
end

render()
setTicking(true)
