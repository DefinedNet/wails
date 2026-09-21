package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var start = time.Now()

func stamp() string {
	return fmt.Sprintf("T+%6.3fs", time.Since(start).Seconds())
}

func logf(format string, args ...any) {
	fmt.Printf("%s %s  %s\n", time.Now().Format("15:04:05.000"), stamp(), fmt.Sprintf(format, args...))
	os.Stdout.Sync()
}

// Template icons are drawn from alpha only, so a filled shape is enough for the
// menu bar to render a recognisable glyph.
func templateIcon(filled func(x, y, size int) bool) []byte {
	const size = 22
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if filled(x, y, size) {
				img.Set(x, y, color.NRGBA{0, 0, 0, 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

func circleIcon() []byte {
	return templateIcon(func(x, y, size int) bool {
		cx, cy, r := float64(size)/2, float64(size)/2, float64(size)/2-2
		dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
		return dx*dx+dy*dy <= r*r
	})
}

func squareIcon() []byte {
	return templateIcon(func(x, y, size int) bool {
		return x >= 3 && x < size-3 && y >= 3 && y < size-3
	})
}

func main() {
	openAt := flag.Duration("open-menu-after", 3*time.Second, "open the tray menu programmatically after this delay; 0 disables")
	dismissAfter := flag.Duration("dismiss-after", 6*time.Second, "dismiss the tracking menu after this delay; 0 leaves it to the user")
	quitAfter := flag.Duration("quit-after", 14*time.Second, "exit after this delay; 0 runs until quit")
	observeTitle := flag.Bool("observe-title", false, "read the status item button title back each tick; the window walk can end menu tracking early")
	interval := flag.Duration("interval", time.Second, "tray update interval")
	flag.Parse()

	app := application.New(application.Options{
		Name:   "systray-menu-tracking-repro",
		Assets: application.AlphaAssets,
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory,
		},
	})

	tray := app.SystemTray.New()
	tray.SetTemplateIcon(circleIcon())
	tray.SetLabel("tick 0")

	menu := app.NewMenu()
	menu.Add("Leave this menu open and watch the tray")
	menu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)

	go func() {
		ticker := time.NewTicker(*interval)
		defer ticker.Stop()
		for tick := 1; ; tick++ {
			select {
			case <-ticker.C:
			case <-app.Context().Done():
				return
			}
			shape := "circle"
			icon := circleIcon()
			if tick%2 == 1 {
				shape, icon = "square", squareIcon()
			}
			logf("call  SetTemplateIcon(%s) + SetLabel(tick %d)", shape, tick)
			tray.SetTemplateIcon(icon)
			tray.SetLabel(fmt.Sprintf("tick %d", tick))
			probe(tick, *observeTitle)
		}
	}()

	if *openAt > 0 {
		go func() {
			time.Sleep(*openAt)
			logf("event OpenMenu() - native menu tracking starts here")
			tray.OpenMenu()
		}()
	}
	if *dismissAfter > 0 && *openAt > 0 {
		go func() {
			time.Sleep(*openAt + *dismissAfter)
			logf("event dismissing menu (synthetic Escape)")
			dismissMenu()
		}()
	}
	if *quitAfter > 0 {
		go func() {
			time.Sleep(*quitAfter)
			logf("event quitting")
			app.Quit()
		}()
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
