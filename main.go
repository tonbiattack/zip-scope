// Command zipscope is the Windows desktop viewer.
package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/tonbiattack/zip-scope/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	dir, err := dataDir()
	if err != nil {
		log.Fatalf("zipscope: %v", err)
	}

	application, err := app.New(dir)
	if err != nil {
		log.Fatalf("zipscope: %v", err)
	}

	// A path on the command line comes from the Explorer context menu, a folder
	// drop, or "Open with". It selects that location instead of restoring the
	// last one.
	if target := startupArg(os.Args[1:]); target != "" {
		application.SetStartupTarget(target)
	}

	cfg := application.GetSettings()

	err = wails.Run(&options.App{
		Title:     "ZipScope",
		Width:     cfg.WindowWidth,
		Height:    cfg.WindowHeight,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// Everything that is not a bundled asset falls through to the image
			// handler, which only understands /img/<validated id>.
			Handler: application.ImageHandler(),
		},
		BackgroundColour: &options.RGBA{R: 18, G: 18, B: 20, A: 255},
		OnStartup:        application.Startup,
		OnShutdown:       application.Shutdown,
		Bind:             []interface{}{application},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})
	if err != nil {
		log.Printf("zipscope: %v", err)
		os.Exit(1)
	}
}

// startupArg returns the first argument that names an existing folder or
// ZIP/CBZ file. Flags and anything else are ignored so an unexpected argument
// cannot make the app try to open an arbitrary path.
func startupArg(args []string) string {
	for _, a := range args {
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		abs, err := filepath.Abs(a)
		if err != nil {
			continue
		}
		fi, err := os.Stat(abs)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			return abs
		}
		switch strings.ToLower(filepath.Ext(abs)) {
		case ".zip", ".cbz":
			return abs
		}
	}
	return ""
}

// dataDir resolves %LOCALAPPDATA%/ZipScope.
func dataDir() (string, error) {
	return app.DataDir()
}
