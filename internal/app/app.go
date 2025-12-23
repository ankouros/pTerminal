package app

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime"

	"github.com/ankouros/pterminal/internal/config"
	"github.com/ankouros/pterminal/internal/model"
	"github.com/ankouros/pterminal/internal/p2p"
	"github.com/ankouros/pterminal/internal/session"
	"github.com/ankouros/pterminal/internal/ui"
)

func Run() error {
	if runtime.GOOS != "linux" {
		return errors.New("pterminal scaffold currently targets linux")
	}

	// Load canonical application config (pterminal.json)
	cfg, cfgPath, err := config.EnsureConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Session manager operates on AppConfig
	mgr := session.NewManager(cfg)

	baseDir := filepath.Dir(cfgPath)
	p2pSvc, err := p2p.NewService(cfg, baseDir)
	if err != nil {
		log.Printf("p2p disabled: %v", err)
		p2pSvc = nil
	}

	w, err := ui.NewWindow(mgr, p2pSvc)
	if err != nil {
		return err
	}
	defer w.Close()
	if p2pSvc != nil {
		defer p2pSvc.Close()
	}

	if p2pSvc != nil {
		p2pSvc.SetOnMerged(func(cfg model.AppConfig) {
			if err := config.Save(cfg); err != nil {
				log.Printf("p2p config save failed: %v", err)
				return
			}
			w.ApplyConfig(cfg)
		})
	}

	w.Run()
	return nil
}
