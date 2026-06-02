package webapp

import (
	"time"

	"containerway/internal/fyneprefs"
	"containerway/internal/webprefs"
)

func sessionIdleExceeded(ws webSession) (bool, int) {
	cfg, err := webprefs.Load()
	if err != nil || cfg.Policies.SessionIdleMinutes <= 0 {
		return false, 0
	}
	mins := cfg.Policies.SessionIdleMinutes
	if ws.LastActivity.IsZero() {
		return false, mins
	}
	if time.Since(ws.LastActivity) > time.Duration(mins)*time.Minute {
		return true, mins
	}
	return false, mins
}

func notifyLoginFailure(cfg webprefs.Settings, username string) error {
	if !cfg.Notifications.LoginFailure {
		return nil
	}
	mail, err := fyneprefs.LoadMail()
	if err != nil || !mail.Valid() {
		return err
	}
	subject := "ContainerWay Web — tentativa de login falhada"
	msg := "Utilizador: " + username + "\nLimite de tentativas atingido."
	return mail.Send(subject, msg)
}
