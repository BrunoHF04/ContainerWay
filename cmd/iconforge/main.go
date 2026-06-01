package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// main gera ícones PNG/ICO para desktop e/ou versão web.
func main() {
	desktop := flag.Bool("desktop", false, "gerar containerway-icon.png/.ico (desktop)")
	web := flag.Bool("web", false, "gerar ícones web, favicon e assets estáticos")
	flag.Parse()
	if !*desktop && !*web {
		*desktop = true
		*web = true
	}

	if err := os.MkdirAll("assets", 0o755); err != nil {
		panic(err)
	}
	staticDir := filepath.Join("internal", "webapp", "static")
	if *web {
		if err := os.MkdirAll(staticDir, 0o755); err != nil {
			panic(err)
		}
	}

	if *desktop {
		icon := buildIcon(256)
		if err := writePNG(icon, filepath.Join("assets", "containerway-icon.png")); err != nil {
			panic(err)
		}
		if err := writeICOMulti(filepath.Join("assets", "containerway-icon.ico"), []int{16, 32, 48, 256}, icon); err != nil {
			panic(err)
		}
		if err := writePNG(scaleIcon(icon, 64), filepath.Join("internal", "appui", "window-icon.png")); err != nil {
			panic(err)
		}
		fmt.Println("desktop: assets/containerway-icon.png, assets/containerway-icon.ico")
	}

	if *web {
		iconDark := buildWebIcon(256)
		iconLight := buildWebIconLight(256)
		if err := writePNG(iconDark, filepath.Join("assets", "containerway-web-icon.png")); err != nil {
			panic(err)
		}
		if err := writeICOMulti(filepath.Join("assets", "containerway-web-icon.ico"), []int{16, 32, 48, 256}, iconDark); err != nil {
			panic(err)
		}
		if err := writeICOMulti(filepath.Join(staticDir, "favicon.ico"), []int{16, 32, 48}, iconDark); err != nil {
			panic(err)
		}
		for _, spec := range []struct {
			name  string
			size  int
			light bool
		}{
			{"favicon-16x16.png", 16, false},
			{"favicon-32x32.png", 32, false},
			{"favicon-32x32-light.png", 32, true},
			{"apple-touch-icon.png", 180, false},
		} {
			src := iconDark
			if spec.light {
				src = iconLight
			}
			if err := writePNG(scaleIcon(src, spec.size), filepath.Join(staticDir, spec.name)); err != nil {
				panic(err)
			}
		}
		for _, spec := range []struct {
			name  string
			size  int
			light bool
		}{
			{"icon-192.png", 192, false},
			{"icon-192-light.png", 192, true},
			{"mascot-hero.png", mascotHeroSize, false},
			{"mascot-hero-light.png", mascotHeroSize, true},
		} {
			hero := buildMascotHero(spec.size, spec.light)
			if err := writePNG(hero, filepath.Join(staticDir, spec.name)); err != nil {
				panic(err)
			}
		}
		winIcon := scaleIcon(iconDark, 64)
		if err := writePNG(winIcon, filepath.Join("internal", "appui", "window-icon.png")); err != nil {
			panic(err)
		}
		fmt.Println("web: assets/containerway-web-icon.* + internal/webapp/static/favicon*")
	}
}
