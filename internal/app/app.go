package app

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/ankouros/pterminal/internal/config"
	"github.com/ankouros/pterminal/internal/session"
	"github.com/ankouros/pterminal/internal/ui"
)

func Run() error {
	if runtime.GOOS != "linux" {
		return errors.New("pterminal scaffold currently targets linux")
	}

	// Load canonical application config (pterminal.json)
	cfg, _, err := config.EnsureConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Session manager operates on AppConfig
	mgr := session.NewManager(cfg)

	w, err := ui.NewWindow(mgr)
	if err != nil {
		return err
	}
	defer w.Close()

	w.Run()
	return nil
}
