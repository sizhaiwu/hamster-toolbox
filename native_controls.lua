-- Hamster Toolbox native media controls. MIT; no third-party controller code.
local assdraw = require 'mp.assdraw'
local overlay = mp.create_osd_overlay('ass-events')
local boxes, dragging, pressed = {}, nil, nil
local w, h, dpi, bar, extra = 0, 0, 1, 48, false
local volume_until, volume_open = 0, false
local accent = 'F5A513'
local top_until, bottom_until = 0, 0
local top_visible, bottom_visible = false, false
local force_visible = false
local mouse_inside = false
local draw
local tick

local function clamp(v, lo, hi) return math.max(lo, math.min(hi, v)) end
local function number(name, fallback) return mp.get_property_number(name, fallback or 0) end
local function pointer()
    local x, y = mp.get_mouse_pos()
    x, y = (x or -1000) / dpi, (y or -1000) / dpi
    return x, y, mouse_inside and x >= 0 and y >= 0 and x <= w and y <= h
end
local function contains(b, x, y)
    return b and x >= b.x and x <= b.x + b.w and y >= b.y and y <= b.y + b.h
end
local function hit(x, y)
    for i = #boxes, 1, -1 do if contains(boxes[i], x, y) then return boxes[i] end end
end
local function clock(value)
    value = math.max(0, math.floor(value or 0))
    if value >= 3600 then return string.format('%d:%02d:%02d', math.floor(value / 3600), math.floor(value / 60) % 60, value % 60) end
    return string.format('%02d:%02d', math.floor(value / 60), value % 60)
end
local function command(...) pcall(mp.commandv, ...) end
local function rect(a, x, y, width, height, color, alpha)
    a:new_event(); a:append('{\\an7\\pos(0,0)\\bord0\\shad0\\1c&H' .. color .. string.format('&\\1a&H%02X&}', alpha or 0))
    a:draw_start(); a:rect_cw(x, y, x + width, y + height); a:draw_stop()
end
local function text(a, value, x, y, width, size, color)
    if width <= 0 then return end
    local escaped = mp.command_native({'escape-ass', tostring(value or '')}) or ''
    a:new_event(); a:append(string.format('{\\an7\\pos(%.2f,%.2f)\\clip(%.2f,%.2f,%.2f,%.2f)\\fnsans-serif\\fs%d\\bord0\\shad0\\1c&H%s&}%s',
        x, y, x, y - 2, x + width, y + size + 8, size, color or 'F4F1EB', escaped))
end
local function box_text(a, value, x, y, width, height, size, color, centered)
    if width <= 0 or height <= 0 then return end
    local escaped = mp.command_native({'escape-ass', tostring(value or '')}) or ''
    -- Anchor labels to their actual box instead of estimating a font baseline.
    a:new_event(); a:append(string.format('{\\an%d\\pos(%.2f,%.2f)\\clip(%.2f,%.2f,%.2f,%.2f)\\fnsans-serif\\fs%d\\bord0\\shad0\\1c&H%s&}%s',
        centered and 5 or 4, centered and x + width / 2 or x, y + height / 2,
        x, y, x + width, y + height, size, color or 'F4F1EB', escaped))
end
local function button(a, id, label, x, y, width, action)
    local b = {id = id, x = x, y = y, w = width, h = 28, action = action}
    boxes[#boxes + 1] = b
    local mx, my = pointer()
    if contains(b, mx, my) then rect(a, x, y, width, 28, '46392B') end
    box_text(a, label, x, y, width, b.h, 15, nil, true)
end
local function circle(a, x, y, radius, color, outline)
    a:new_event()
    if outline then a:append('{\\an7\\pos(0,0)\\bord1.5\\shad0\\1a&HFF&\\3a&H00&\\3c&H' .. color .. '&}')
    else a:append('{\\an7\\pos(0,0)\\bord0\\shad0\\1a&H00&\\1c&H' .. color .. '&}') end
    a:draw_start()
    for i = 0, 32 do
        local angle = i * math.pi / 16
        local px, py = x + math.cos(angle) * radius, y + math.sin(angle) * radius
        if i == 0 then a:move_to(px, py) else a:line_to(px, py) end
    end
    a:draw_stop()
end
local function loop_points()
    return mp.get_property_number('ab-loop-a'), mp.get_property_number('ab-loop-b')
end
local function loop_message(value)
    mp.osd_message(value, 2)
    bottom_until = mp.get_time() + 2
    if draw then draw() end
end
local function clear_loop()
    mp.set_property('ab-loop-b', 'no')
    mp.set_property('ab-loop-a', 'no')
end
local function set_loop_point(point)
    if mp.get_property_bool('current-tracks/video/image', false) or not mp.get_property_bool('seekable', false) then
        loop_message('AB 重复仅用于可定位的视频'); return
    end
    if point == 'clear' then clear_loop(); loop_message('已取消 AB 重复'); return end
    local pos = mp.get_property_number('time-pos')
    if not pos then return end
    local a = loop_points()
    if point == 'a' then
        if mp.get_property_bool('eof-reached', false) then loop_message('请先跳转到视频结束前'); return end
        -- A starts a fresh range; never keep a stale B from the previous range.
        mp.set_property('ab-loop-b', 'no')
        mp.set_property_number('ab-loop-a', pos)
        loop_message('A 点：' .. clock(pos))
    elseif not a then
        loop_message('请先按 Ctrl+1 设置 A 点')
    elseif pos <= a then
        loop_message('B 点必须晚于 A 点')
    else
        -- mpv performs the loop in its playback engine, without a polling seek.
        mp.set_property('ab-loop-count', 'inf')
        mp.set_property_number('ab-loop-b', pos)
        loop_message('B 点：' .. clock(pos) .. '\nAB 重复播放中 · Ctrl+3 取消')
    end
end
local function loop_markers(a, y, duration)
    if duration <= 0 then return end
    local first, last = loop_points()
    local function position(value) return clamp(value / duration * w, 8, w - 8) end
    if first and last then
        local left, right = clamp(first / duration * w, 0, w), clamp(last / duration * w, 0, w)
        rect(a, math.min(left, right), y + 3, math.abs(right - left), 7, accent, 120)
        if math.abs(position(last) - position(first)) < 18 then
            local x = clamp((position(first) + position(last)) / 2, 14, w - 14)
            rect(a, x - 13, y - 2, 26, 17, '302B26')
            box_text(a, 'AB', x - 13, y - 2, 26, 17, 11, accent, true)
            return
        end
    end
    for _, entry in ipairs({{first, 'A'}, {last, 'B'}}) do
        if entry[1] then
            local x = position(entry[1])
            circle(a, x, y + 6.5, 7, '302B26')
            circle(a, x, y + 6.5, 7, accent, true)
            box_text(a, entry[2], x - 7, y - 0.5, 14, 14, 10, 'F4F1EB', true)
        end
    end
end
local function play_button(a, x, y)
    button(a, 'play', '', x, y, 30, function()
        if mp.get_property_bool('eof-reached', false) then
            command('seek', 0, 'absolute+exact')
            mp.set_property_bool('pause', false)
        else
            command('cycle', 'pause')
        end
    end)
    circle(a, x + 15, y + 14, 14, accent, true)
    if mp.get_property_bool('pause') or mp.get_property_bool('eof-reached', false) then
        a:new_event(); a:append('{\\an7\\pos(0,0)\\bord0\\shad0\\1a&H00&\\1c&H' .. accent .. '&}')
        a:draw_start(); a:move_to(x + 11, y + 7); a:line_to(x + 22, y + 14); a:line_to(x + 11, y + 22); a:draw_stop()
    else
        rect(a, x + 10, y + 7, 3, 14, accent); rect(a, x + 18, y + 7, 3, 14, accent)
    end
end
local function speaker(a, x, y, muted)
    button(a, 'mute', '', x, y, 28, function() command('cycle', 'mute') end)
    local color = muted and 'B3A89D' or accent
    a:new_event(); a:append('{\\an7\\pos(0,0)\\bord0\\shad0\\1a&H00&\\1c&H' .. color .. '&}')
    a:draw_start();a:move_to(x + 4,y + 10);a:line_to(x + 9,y + 10);a:line_to(x + 15,y + 5);a:line_to(x + 15,y + 23);a:line_to(x + 9,y + 18);a:line_to(x + 4,y + 18);a:draw_stop()
    if muted then box_text(a, '×', x + 16, y, 12, 28, 16, color, true)
    else
        for _, r in ipairs({6, 10}) do
            a:new_event();a:append('{\\an7\\pos(0,0)\\bord0\\shad0\\1a&H00&\\1c&H' .. color .. '&}')
            a:draw_start()
            for i=0,12 do local angle=(-60+i*10)*math.pi/180;local px,py=x+15+math.cos(angle)*r,y+14+math.sin(angle)*r;if i==0 then a:move_to(px,py)else a:line_to(px,py)end end
            for i=12,0,-1 do local angle=(-60+i*10)*math.pi/180;a:line_to(x+15+math.cos(angle)*(r-1.5),y+14+math.sin(angle)*(r-1.5))end
            a:draw_stop()
        end
    end
end
local function fullscreen_button(a, x, y)
    button(a, 'fullscreen', '', x, y, 32, function() command('cycle', 'fullscreen') end)
    for _, offset in ipairs({{6,5,1,1},{25,5,-1,1},{6,23,1,-1},{25,23,-1,-1}})do
        local xx,yy=x+offset[1],y+offset[2]
        rect(a, math.min(xx,xx+offset[3]*7), yy, 8, 1.5, 'F4F1EB')
        rect(a, xx, math.min(yy,yy+offset[4]*7), 1.5, 8, 'F4F1EB')
    end
end
local function seek(b, x)
    local duration = number('duration')
    if duration > 0 then command('seek', clamp((x - b.x) / b.w, 0, 1) * duration, 'absolute+exact') end
end
local function volume(b, x, y)
    mp.set_property_number('volume', clamp((b.y + b.h - y) / b.h, 0, 1) * 100)
end
local function pointer_move()
    mouse_inside = true
    if w <= 0 or h <= 0 then return end
    local x, y, hover = pointer()
    if not hover then dragging, pressed = nil, nil; top_until, bottom_until = 0, 0
    else
        local now = mp.get_time()
        if y <= 40 then top_until = now + 1 end
        if y >= h - bar - 8 or dragging then bottom_until = now + 1 end
        if dragging then if dragging.id == 'seek' then seek(dragging, x) else volume(dragging, x, y) end end
    end
    draw()
end
local function down()
    local x, y = pointer(); local b = hit(x, y); pressed = b
    if b and (b.id == 'seek' or b.id == 'volume') then
        dragging = b
        if b.id == 'seek' then seek(b, x) else volume(b, x, y) end
    end
    draw()
end
local function up()
    local x, y = pointer(); local b = hit(x, y)
    if pressed and b and pressed.id == b.id and pressed.action then pressed.action() end
    dragging, pressed = nil, nil; draw()
end
local function wheel(delta)
    local x, y = pointer(); local b = hit(x, y)
    if b and b.id == 'seek' then command('seek', delta * 5, 'relative')
    else mp.set_property_number('volume', clamp(number('volume', 100) + delta * 5, 0, 100)) end
    draw()
end
draw = function()
    local width, height = mp.get_osd_size()
    if not width or not height or width <= 0 or height <= 0 then return end
    dpi = clamp(number('display-hidpi-scale', 1), 1, 2.5)
    w, h = width / dpi, height / dpi
    local image = mp.get_property_bool('current-tracks/video/image', false)
    extra = not image and w >= 220
    bar = image and 36 or 48
    local now = mp.get_time()
    local mx, my, inside = pointer()
    local mute_x = w - (extra and 134 or 40)
    local popup_h = math.min(116, h - bar - 8)
    local popup = {x=clamp(mute_x-8,6,w-50),y=h-bar-popup_h-4,w=44,h=popup_h}
    local over_volume = inside and contains({x=mute_x,y=h-31,w=28,h=28},mx,my)
    local over_popup = inside and volume_open and contains(popup,mx,my)
    if over_volume or over_popup or (dragging and dragging.id=='volume') then volume_until=now+0.35 end
    volume_open = not image and popup_h>=40 and now<volume_until
    top_visible = force_visible or (inside and my <= 32) or now < top_until
    bottom_visible = force_visible or dragging ~= nil or volume_open or (inside and my >= h - bar) or now < bottom_until
    overlay.res_x, overlay.res_y = math.floor(w + 0.5), math.floor(h + 0.5)
    boxes = {}
    local a = assdraw.ass_new()
    if top_visible then
        rect(a, 0, 0, w, 32, '201710', 90)
        local title = mp.get_property('media-title', mp.get_property('filename', '仓鼠播放器'))
        text(a, title, 9, 6, math.max(0, w - 116), 15, 'D9D3CC')
        button(a, 'minimize', '−', w - 102, 2, 32, function() command('cycle', 'window-minimized') end)
        button(a, 'maximize', '□', w - 68, 2, 32, function() command('cycle', 'window-maximized') end)
        button(a, 'close', '×', w - 34, 2, 32, function() command('quit') end)
    end
    if bottom_visible and image then
        local y = h - bar
        rect(a, 0, y, w, bar, '201710', 110)
        local actions = {
            {'image-out', '缩小', function() command('add', 'video-zoom', -0.25) end},
            {'image-in', '放大', function() command('add', 'video-zoom', 0.25) end},
            {'image-reset', '复位', function() mp.set_property_number('video-zoom', 0); mp.set_property_number('video-pan-x', 0); mp.set_property_number('video-pan-y', 0) end},
            {'image-rotate', '旋转', function() mp.set_property_number('video-rotate', (number('video-rotate') + 90) % 360) end},
        }
        for i, entry in ipairs(actions) do local x = 8 + (i - 1) * 50; if x + 46 <= w then button(a, entry[1], entry[2], x, y + 4, 46, entry[3]) end end
    elseif bottom_visible then
        local y = h - bar
        rect(a, 0, y, w, bar, '201710', 145)
        local duration, pos = number('duration'), number('time-pos')
        local sb = {id = 'seek', x = 0, y = y, w = math.max(16,w), h = 14}
        boxes[#boxes+1]=sb
        local progress=sb.w*clamp(duration>0 and pos/duration or 0,0,1)
        rect(a, 0, y+5, w, 3, '69645E', 40)
        rect(a, 0, y+5, progress, 3, accent)
        circle(a,clamp(progress,4,w-4),y+6.5,4,accent)
        loop_markers(a,y,duration)
        local row = y+17
        play_button(a,8,row)
        local available=math.max(0,mute_x-60)
        if available>=40 then
            local value=available>=130 and clock(pos)..' / '..clock(duration)or clock(pos)
            box_text(a,value,50,row,available,28,14,'E5DBCE',false)
        end
        local muted=mp.get_property_bool('mute')
        speaker(a,mute_x,row,muted)
        if extra then
            button(a,'speed',string.format('%gx',number('speed',1)),mute_x+36,row,50,function()
                local current=number('speed',1);mp.set_property_number('speed',current>=2 and 0.75 or current+0.25)
            end)
            fullscreen_button(a,w-40,row)
        end
        if volume_open then
            rect(a,popup.x,popup.y,popup.w,popup.h,'302B26',25)
            local vb={id='volume',x=popup.x+5,y=popup.y+9,w=popup.w-10,h=popup.h-35}
            boxes[#boxes+1]=vb
            local level=clamp(number('volume',100),0,100)
            local xx=popup.x+popup.w/2
            local yy=vb.y+vb.h*(1-level/100)
            rect(a,xx-1.5,vb.y,3,vb.h,'77716A')
            rect(a,xx-1.5,yy,3,vb.y+vb.h-yy,accent)
            circle(a,xx,yy,4.5,accent)
            box_text(a,string.format('%d%%',math.floor(level+0.5)),popup.x,popup.y+popup.h-24,popup.w,24,12,nil,true)
        end
    end
    overlay.data = a.text; overlay:update()
    mp.set_mouse_area(0, 0, width, height, 'hamster-watch')
    mp.set_mouse_area(0, bottom_visible and (h - bar) * dpi or 0, bottom_visible and width or 0, bottom_visible and height or 0, 'hamster-bottom')
    mp.set_mouse_area(top_visible and math.max(0, w - 102) * dpi or 0, 0, top_visible and width or 0, top_visible and 32 * dpi or 0, 'hamster-top')
    mp.set_mouse_area(volume_open and popup.x*dpi or 0,volume_open and popup.y*dpi or 0,volume_open and (popup.x+popup.w)*dpi or 0,volume_open and (popup.y+popup.h)*dpi or 0,'hamster-volume')
    -- Read-only layout metadata supports diagnostics and real-player tests.
    local layout = {kind = image and 'image' or 'video', mode = 'full', width = w, height = h, font = 15 * dpi, scale = dpi, bottom = bottom_visible, top = top_visible, bar = bar, controls = {}}
    for _, b in ipairs(boxes) do layout.controls[#layout.controls + 1] = {id = b.id, x = b.x * dpi, y = b.y * dpi, w = b.w * dpi, h = b.h * dpi} end
    pcall(mp.set_property_native, 'user-data/hamster-controls', layout)
    if tick then if top_visible or bottom_visible then tick:resume() else tick:kill() end end
end

mp.add_forced_key_binding('Ctrl+1', 'hamster-loop-a', function() set_loop_point('a') end)
mp.add_forced_key_binding('Ctrl+2', 'hamster-loop-b', function() set_loop_point('b') end)
mp.add_forced_key_binding('Ctrl+3', 'hamster-loop-clear', function() set_loop_point('clear') end)
tick = mp.add_periodic_timer(0.15, function() draw() end); tick:kill()
mp.set_key_bindings({{'mouse_move', pointer_move}, {'mouse_leave', function() mouse_inside = false; volume_until=0;volume_open=false;top_until, bottom_until = 0, 0; dragging, pressed = nil, nil; draw() end}}, 'hamster-watch', 'force')
mp.enable_key_bindings('hamster-watch', 'allow-vo-dragging+allow-hide-cursor')
for _, section in ipairs({'hamster-bottom', 'hamster-top', 'hamster-volume'}) do
    mp.set_key_bindings({{'mbtn_left', up, down}, {'mbtn_left_dbl', 'ignore'}, {'wheel_up', function() wheel(1) end}, {'wheel_down', function() wheel(-1) end}}, section, 'force')
    mp.enable_key_bindings(section)
end
for _, name in ipairs({'osd-dimensions', 'display-hidpi-scale', 'pause', 'eof-reached', 'mute', 'volume', 'duration', 'speed', 'media-title', 'fullscreen', 'current-tracks/video/image', 'video-zoom', 'video-rotate', 'ab-loop-a', 'ab-loop-b'}) do
    mp.observe_property(name, 'native', function() draw() end)
end
mp.register_script_message('hamster-controls-show', function(value) force_visible = value == 'yes'; draw() end)
mp.register_event('shutdown', function() overlay:remove() end)
mp.register_event('file-loaded', function() clear_loop(); draw() end)
