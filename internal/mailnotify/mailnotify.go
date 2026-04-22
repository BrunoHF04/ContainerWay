// Package mailnotify envia e-mails simples via SMTP (STARTTLS / TLS implícito),
// para alertas administrativos do ContainerWay.
package mailnotify

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

// Settings agrupa a configuração usada para envio.
type Settings struct {
	Enabled    bool
	Host       string
	Port       int
	User       string
	Password   string
	From       string
	Recipients []string
}

func resolvedFromAddr(s Settings) string {
	from := strings.TrimSpace(s.From)
	if from == "" {
		from = strings.TrimSpace(s.User)
	}
	return from
}

// EnvelopeFromAddress devolve o endereço usado no MAIL FROM (campo From ou usuário SMTP).
func (s Settings) EnvelopeFromAddress() string {
	return resolvedFromAddr(s)
}

// ValidTransport verifica se SMTP está configurado o suficiente para autenticar e enviar
// (sem exigir destinatários na lista — usado no teste só no remetente).
func (s Settings) ValidTransport() bool {
	if !s.Enabled {
		return false
	}
	if strings.TrimSpace(s.Host) == "" || s.Port <= 0 {
		return false
	}
	if strings.TrimSpace(s.User) == "" {
		return false
	}
	if smtpPasswordForAuth(s.Password) == "" {
		return false
	}
	addr := resolvedFromAddr(s)
	return addr != "" && strings.Contains(addr, "@")
}

// Valid indica se a configuração mínima para envio aos destinatários cadastrados está completa.
func (s Settings) Valid() bool {
	if !s.ValidTransport() {
		return false
	}
	rec := normalizedRecipients(s.Recipients)
	if len(rec) == 0 {
		return false
	}
	for _, e := range rec {
		if !strings.Contains(e, "@") {
			return false
		}
	}
	return true
}

// NormalizeRecipients remove espaços, entradas vazias e duplicatas (sem distinção de maiúsculas).
func NormalizeRecipients(in []string) []string {
	return normalizedRecipients(in)
}

func normalizedRecipients(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		e := strings.TrimSpace(raw)
		if e == "" {
			continue
		}
		key := strings.ToLower(e)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, e)
	}
	return out
}

// smtpPasswordForAuth normaliza a senha SMTP: trim e remove espaços (ex.: senhas de app do Google
// costumam ser exibidas em grupos de 4 caracteres com espaço, mas a autenticação usa só os 16 caracteres).
func smtpPasswordForAuth(p string) string {
	return strings.ReplaceAll(strings.TrimSpace(p), " ", "")
}

func messageIDDomain(from string) string {
	if i := strings.LastIndex(from, "@"); i >= 0 && i < len(from)-1 {
		return strings.TrimSpace(from[i+1:])
	}
	return "localhost"
}

func formatFromHeader(envelopeEmail string) string {
	email := strings.TrimSpace(envelopeEmail)
	if email == "" {
		return ""
	}
	if strings.Contains(email, "<") {
		return email
	}
	return fmt.Sprintf("ContainerWay <%s>", email)
}

// smtpDotStuffCRLF aplica RFC 5321 (DATA): qualquer linha do corpo que comece com '.' deve
// ser prefixada com outro '.'; caso contrário o servidor pode truncar a mensagem ou rejeitar.
func smtpDotStuffCRLF(body string) string {
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\r\n")
	for i, line := range lines {
		if strings.HasPrefix(line, ".") {
			lines[i] = "." + line
		}
	}
	return strings.Join(lines, "\r\n")
}

func buildMessage(envelopeFrom, toHeader, subject, body string) []byte {
	domain := messageIDDomain(envelopeFrom)
	idRand := make([]byte, 10)
	_, _ = rand.Read(idRand)
	msgID := fmt.Sprintf("<%s-%d@%s>", hex.EncodeToString(idRand), time.Now().UnixNano(), domain)

	bodyCRLF := smtpDotStuffCRLF(body)

	var b strings.Builder
	b.WriteString("From: " + formatFromHeader(envelopeFrom) + "\r\n")
	b.WriteString("To: " + toHeader + "\r\n")
	b.WriteString("Subject: " + strings.TrimSpace(subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: " + msgID + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(bodyCRLF)
	return []byte(b.String())
}

func (s Settings) send(envelopeFrom string, rec []string, subject, body string) error {
	rec = normalizedRecipients(rec)
	if len(rec) == 0 {
		return fmt.Errorf("nenhum destinatário")
	}
	host := strings.TrimSpace(s.Host)
	addr := fmt.Sprintf("%s:%d", host, s.Port)
	toHeader := strings.Join(rec, ", ")
	msg := buildMessage(envelopeFrom, toHeader, strings.TrimSpace(subject), body)

	var auth smtp.Auth
	if strings.TrimSpace(s.User) != "" {
		auth = smtp.PlainAuth("", strings.TrimSpace(s.User), smtpPasswordForAuth(s.Password), host)
	}
	return smtp.SendMail(addr, auth, envelopeFrom, rec, msg)
}

// Send envia uma mensagem de texto aos destinatários cadastrados em Recipients.
func (s Settings) Send(subject, body string) error {
	if !s.Valid() {
		return fmt.Errorf("configuração de e-mail incompleta ou desativada")
	}
	envelopeFrom := resolvedFromAddr(s)
	return s.send(envelopeFrom, normalizedRecipients(s.Recipients), subject, body)
}

// SendTestToSelf envia só para o endereço do remetente (From / usuário SMTP), útil para
// confirmar que o Gmail aceita o envio quando o e-mail corporativo bloqueia destinos externos.
func (s Settings) SendTestToSelf(subject, body string) error {
	if !s.ValidTransport() {
		return fmt.Errorf("preencha ativar envio, servidor, porta, usuário, senha e remetente (From) válidos")
	}
	envelopeFrom := resolvedFromAddr(s)
	return s.send(envelopeFrom, []string{envelopeFrom}, subject, body)
}
