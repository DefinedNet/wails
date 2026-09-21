# macOS: system tray icon and label updates are not delivered while the menu is open

A menu tray whose menu was opened through `ShowMenu()` / `OpenMenu()` stops
updating. Every `SetIcon`, `SetTemplateIcon` and `SetLabel` call issued while
the menu is open is delivered only after the menu is dismissed, and then all at
once.

Filed as wailsapp/wails#6154. The right click dismissal defect found while
measuring it is wailsapp/wails#6155.

Tested on macOS 27.0 (26A428), arm64, against `v3` at `master` (`4b1f3484`).

## Run it

```
go run . -interval 250ms -quit-after 12s -dismiss-after 6s
```

The app builds against the checked out `v3` tree through the `replace`
directive in `go.mod`. It needs no bundle and no permissions.

It creates a menu tray, alternates a circle and a square template icon plus a
`tick N` label every interval, opens its own menu after `-open-menu-after`, and
dismisses it with a posted Escape after `-dismiss-after`. Use
`-open-menu-after 0 -quit-after 60s` to click the tray by hand instead.

Every tick also schedules two probe blocks on the main thread, one through
`dispatch_async(dispatch_get_main_queue(), ...)` (what `systemtray_darwin.m`
uses) and one through `CFRunLoopPerformBlock(kCFRunLoopCommonModes)` (what
`mainthread_darwin.go` uses since #6026), and logs the current main run loop
mode. The mode line proves a nested tracking loop was really active.

`-observe-title` reads the status item button title back from the main thread.
It shows the visible label going stale, but walking `[NSApp windows]` during
tracking often ends tracking after a second, so it is off by default.

## Result

Native menu tracking is entered from a main queue drain, so the queue is not
drained again until the menu closes:

```
trace showMenu enqueued on the main queue
trace showMenu block started draining on the main queue
trace showMenu entering mouseDown: (blocks for the menu lifetime)
```

While that block waits in `mouseDown:`, run loop blocks keep arriving on time
and queue blocks do not arrive at all:

```
T+ 3.286s  mode  main run loop is in NSEventTrackingRunLoopMode at tick 13
T+ 3.290s  ran   CFRunLoopPerformBlock(common)  scheduled with tick 13, 0.004s late
...
T+ 8.293s  ran   CFRunLoopPerformBlock(common)  scheduled with tick 33, 0.005s late
trace menuDidClose:
trace showMenu mouseDown: returned, menu dismissed
trace systemTraySetIcon block ran
trace systemTraySetLabel block ran
T+ 8.321s  ran   dispatch_async(main queue)     scheduled with tick 13, 5.029s late
trace systemTraySetIcon block ran
trace systemTraySetLabel block ran
T+ 8.326s  ran   dispatch_async(main queue)     scheduled with tick 14, 4.791s late
```

Every `systemTraySetIcon` and `systemTraySetLabel` block for the tracking
window runs in that burst. The visible icon and label are frozen for the whole
time the menu is open, then jump to the newest value.

Expected: updates issued while the menu is open reach the tray while it is
open, the way main thread work dispatched from Go does since #6026.

Full transcripts are in `logs/`.

## Variants

`trace.patch` adds the `trace` lines above to
`v3/pkg/application/systemtray_darwin.m`. Apply it, rebuild, and the ordering
of the click path is visible in the log.

`cfrunloop-fix.patch` routes all five `dispatch_async(dispatch_get_main_queue())`
sites in `systemtray_darwin.m` through `CFRunLoopPerformBlock` plus
`CFRunLoopWakeUp`, like `dispatchOnMainThread`. With it applied the stall is
gone: during `NSEventTrackingRunLoopMode` the tray keeps counting and the title
read-back follows the tick (`logs/patched-all-sites.log`).

Patching `showMenu` alone is not enough, and is worse: tracking then starts
inside a run loop block callout, and blocks added afterwards do not run until
that callout returns, so the Go caller blocks in `SetTemplateIcon` for the whole
menu lifetime (`logs/patched-showmenu-only.log`). The all-sites patch is
reported as data, not as a proposed fix.

## Real clicks

Ten menu sessions opened by hand, both buttons, on macOS 27.0
(`logs/real-clicks-traced.log`). No session delivered a single
`systemTraySetIcon` or `systemTraySetLabel` block while the menu was open.

| session | button | open for | tray updates during tracking | tray updates within 60 ms of `menuDidClose:` | event monitor fired |
| --- | --- | --- | --- | --- | --- |
| 1 | left | 2.93 s | 0 | 12 | no |
| 2 | right | 2.54 s | 0 | 10 | yes |
| 3 | right | 0.94 s | 0 | 4 | yes |
| 4 | right | 0.68 s | 0 | 3 | yes |
| 5 | right | 0.45 s | 0 | 2 | yes |
| 6 | right | 0.47 s | 0 | 2 | yes |
| 7 | right | 0.51 s | 0 | 2 | yes |
| 8 | right | 0.43 s | 0 | 2 | yes |
| 9 | right | 0.95 s | 0 | 4 | yes |
| 10 | left | 0.74 s | 0 | 3 | no |

Every session, left or right, reaches the menu through `showMenu`:

```
17:20:18.384  trace statusItemClicked: action fired
17:20:18.384  trace showMenu enqueued on the main queue
17:20:18.384  trace showMenu block started draining on the main queue
17:20:18.385  trace showMenu entering mouseDown: (blocks for the menu lifetime)
17:20:18.454  T+ 9.030s  mode  main run loop is in NSEventTrackingRunLoopMode at tick 36
17:20:18.454  T+ 9.030s  ran   CFRunLoopPerformBlock(common)  scheduled with tick 36, 0.000s late
...
17:20:21.313  trace menuDidClose:
17:20:21.314  trace showMenu mouseDown: returned, menu dismissed
17:20:21.318  trace systemTraySetIcon block ran
17:20:21.319  T+11.894s  ran   dispatch_async(main queue)  scheduled with tick 36, 2.865s late
```

The local event monitor never took over tracking in this session: the log has
no `event monitor assigned statusItem.menu` line at all. The reason differs per
button.

Right click: `applySmartDefaults` sets `rightClickHandler = ShowMenu` for every
tray that has a menu, and `systrayPreClickCallback` returns 1 only when the
matching handler is nil, so the monitor's native tracking path is unreachable
for right click on a menu tray, on every macOS version.

Left click: for a pure menu tray (`clickHandler == nil`, no attached window)
`systrayPreClickCallback` does return 1, so the monitor path is reachable by
design. On macOS 26 and earlier the monitor receives the left mouse down,
assigns `statusItem.menu`, and tracking is entered from the event loop rather
than from a queue drain, which does not starve. On macOS 27 the monitor never
receives left mouse down, so the action handler runs `ShowMenu` and left click
starves like right click. The left click sessions above have no monitor line;
the right click sessions do.

On macOS 26 this repro therefore stalls on right click only. On macOS 27 both
buttons stall.

## Second defect: right click cannot dismiss the menu

Right clicking an open menu closes it and reopens it 18-24 ms later, seven
times in this log. Left clicks never reopen. See
`proposed-issue-right-click-reopen.md`.
