// Spike: Wails (github.com/wailsapp/wails/v2) — «сетка + блоки + коннектор + pan/zoom».
// Маска заданий идентична spike/gio и spike/fyne. Вся интерактивность — HTML canvas,
// Go выступает только окном и asset-сервером (мост Go<->JS для спайка не нужен).
package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed frontend/dist
var assets embed.FS

func main() {
	sub, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	err = wails.Run(&options.App{
		Title:  "Spike Wails",
		Width:  1100,
		Height: 760,
		LogLevel: logger.DEBUG,
		Logger:   logger.NewFileLogger("wails.log"),
		AssetServer: &assetserver.Options{
			Assets: sub,
		},
		Bind: []interface{}{},
	})
	if err != nil {
		log.Fatal(err)
	}
}
