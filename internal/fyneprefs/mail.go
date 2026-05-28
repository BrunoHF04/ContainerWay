package fyneprefs

import (
	"encoding/json"
	"strconv"
	"strings"

	"containerway/internal/mailnotify"
)

const (
	KeyNotifyEnabled    = "notify.email.enabled"
	KeyNotifyToLegacy   = "notify.email.to"
	KeyNotifyRecipients = "notify.email.recipients"
	KeySMTPHost         = "notify.smtp.host"
	KeySMTPPort         = "notify.smtp.port"
	KeySMTPUser         = "notify.smtp.user"
	KeySMTPPassword     = "notify.smtp.password"
	KeySMTPFrom         = "notify.smtp.from"
)

// LoadMail devolve configuração SMTP/alertas.
func LoadMail() (mailnotify.Settings, error) {
	m, err := loadMap()
	if err != nil {
		return mailnotify.Settings{}, err
	}
	port, _ := strconv.Atoi(strings.TrimSpace(getString(m, KeySMTPPort, "587")))
	if port <= 0 {
		port = 587
	}
	return mailnotify.Settings{
		Enabled:    getBool(m, KeyNotifyEnabled),
		Host:       strings.TrimSpace(getString(m, KeySMTPHost, "")),
		Port:       port,
		User:       strings.TrimSpace(getString(m, KeySMTPUser, "")),
		Password:   getString(m, KeySMTPPassword, ""),
		From:       strings.TrimSpace(getString(m, KeySMTPFrom, "")),
		Recipients: loadRecipients(m),
	}, nil
}

func loadRecipients(m map[string]json.RawMessage) []string {
	raw := m[KeyNotifyRecipients]
	if len(raw) > 0 {
		var emails []string
		var asString string
		if err := json.Unmarshal(raw, &asString); err == nil && strings.TrimSpace(asString) != "" {
			_ = json.Unmarshal([]byte(asString), &emails)
		} else {
			_ = json.Unmarshal(raw, &emails)
		}
		if emails != nil {
			return mailnotify.NormalizeRecipients(emails)
		}
	}
	legacy := strings.TrimSpace(getString(m, KeyNotifyToLegacy, ""))
	if legacy == "" {
		return nil
	}
	var parts []string
	for _, seg := range strings.Split(legacy, ",") {
		if t := strings.TrimSpace(seg); t != "" {
			parts = append(parts, t)
		}
	}
	return mailnotify.NormalizeRecipients(parts)
}

// SaveMail grava configuração SMTP/alertas.
func SaveMail(s mailnotify.Settings) error {
	m, err := loadMap()
	if err != nil {
		return err
	}
	setBool(m, KeyNotifyEnabled, s.Enabled)
	rec := mailnotify.NormalizeRecipients(s.Recipients)
	if buf, err := json.Marshal(rec); err == nil {
		m[KeyNotifyRecipients] = buf
	}
	if len(rec) > 0 {
		setString(m, KeyNotifyToLegacy, rec[0])
	} else {
		setString(m, KeyNotifyToLegacy, "")
	}
	setString(m, KeySMTPHost, strings.TrimSpace(s.Host))
	setString(m, KeySMTPPort, strconv.Itoa(s.Port))
	setString(m, KeySMTPUser, strings.TrimSpace(s.User))
	setString(m, KeySMTPPassword, s.Password)
	setString(m, KeySMTPFrom, strings.TrimSpace(s.From))
	return saveMap(m)
}
