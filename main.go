// Command zipscope is the Windows desktop viewer.
package main

import (
	"embed"
	"log"
	"os"

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

// dataDir resolves %LOCALAPPDATA%/ZipScope.
func dataDir() (string, error) {
	return app.DataDir()
}
