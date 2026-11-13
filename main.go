package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// The compiled React bundle is embedded into the desktop binary.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	application := NewApp(appVersion)
	windowsOptions := &windows.Options{}
	if webviewData := os.Getenv("HYSQL_WEBVIEW_DATA"); webviewData != "" {
		// Enables an isolated WebView2 profile for debugging and UI automation.
		windowsOptions.WebviewUserDataPath = webviewData
	}
	err := wails.Run(&options.App{
		Title:             "HySQL",
		Width:             1480,
		Height:            920,
		MinWidth:          1180,
		MinHeight:         700,
		DisableResize:     false,
		Fullscreen:        false,
		Frameless:         false,
		StartHidden:       false,
		HideWindowOnClose: false,
		BackgroundColour:  &options.RGBA{R: 9, G: 13, B: 18, A: 255},
		AssetServer:       &assetserver.Options{Assets: assets},
		OnStartup:         application.startup,
		OnShutdown:        application.shutdown,
		Bind:              []interface{}{application},
		Windows:           windowsOptions,
	})
	if err != nil {
		log.Fatal(err)
	}
}
