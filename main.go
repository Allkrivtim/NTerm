package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	var app *App
	var err error
	if isNewWindowProcess(os.Args[1:]) {
		app, err = NewWindowApp()
	} else {
		app, err = NewApp()
	}
	if err != nil {
		log.Fatal(err)
	}
	applicationMenu := menu.NewMenu()
	applicationMenu.Append(menu.AppMenu())
	fileMenu := applicationMenu.AddSubmenu("File")
	fileMenu.AddText("New Window", keys.CmdOrCtrl("n"), func(*menu.CallbackData) {
		if err := app.NewWindow(); err != nil {
			log.Printf("new window: %v", err)
		}
	})
	applicationMenu.Append(menu.EditMenu())
	applicationMenu.Append(menu.WindowMenu())
	err = wails.Run(&options.App{
		Title:            "NTerm",
		Width:            1120,
		Height:           760,
		MinWidth:         560,
		MinHeight:        420,
		DisableResize:    false,
		Fullscreen:       false,
		Frameless:        false,
		BackgroundColour: &options.RGBA{R: 20, G: 20, B: 21, A: 0},
		AssetServer:      &assetserver.Options{Assets: assets},
		Menu:             applicationMenu,
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHidden(),
			Appearance:           mac.DefaultAppearance,
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
		},
		OnStartup:  app.startup,
		OnDomReady: app.domReady,
		OnShutdown: app.shutdown,
		Bind:       []interface{}{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
