package webapp

import (
	"sync"
	"time"

	"containerway/internal/session"
	"containerway/internal/transfer"
)

// sshBundle agrupa sessão remota e estado de transferências da sessão web.
type sshBundle struct {
	Sess *session.Session
	Host string
	User string
	TM   transfer.Manager

	transferMu sync.Mutex
	transferLog []transferRecord
}

type transferRecord struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"` // running, ok, error
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// addTransferLog regista evento de transferência (mantém últimos 50).
func (b *sshBundle) addTransferLog(name, status, errMsg string) {
	b.transferMu.Lock()
	defer b.transferMu.Unlock()
	b.transferLog = append(b.transferLog, transferRecord{
		Name:      name,
		Status:    status,
		Error:     errMsg,
		UpdatedAt: time.Now(),
	})
	if len(b.transferLog) > 50 {
		b.transferLog = b.transferLog[len(b.transferLog)-50:]
	}
}

// transferStatus devolve fila e histórico recente.
func (b *sshBundle) transferStatus() map[string]any {
	b.transferMu.Lock()
	logCopy := append([]transferRecord(nil), b.transferLog...)
	b.transferMu.Unlock()
	return map[string]any{
		"queued":  b.TM.Queued(),
		"running": b.TM.Running(),
		"recent":  logCopy,
	}
}
