package webapp



import (

	"sync"

	"time"



	"containerway/internal/automation"

	"containerway/internal/session"

	"containerway/internal/transfer"

)



// sshBundle agrupa sessão remota e estado de transferências da sessão web.

type sshBundle struct {

	Sess *session.Session

	Host string

	User string

	TM   transfer.Manager



	transferMu  sync.Mutex

	transferLog []transferRecord

	activeXfer  *transferActive



	autoEngine automation.Engine

	clipboardMu sync.Mutex
	clipboard   *clipboardEntry

	parallelJobs int

	sudoMu          sync.Mutex
	sudoEnabled     bool
	sudoUser        string
	sudoPass        string
	sudoValidatedAt time.Time

	externalMu     sync.Mutex
	externalEdits  map[string]*externalEditSession

	opMu    sync.Mutex
	opLog   []operationRecord
}

const sudoSessionTTL = 10 * time.Minute

type externalEditSession struct {
	TempPath    string
	RemotePath  string
	ContainerID string
	LastSynced  time.Time
}

type operationRecord struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
	Level   string    `json:"level"` // info, error
}



type transferRecord struct {

	Name      string    `json:"name"`

	Status    string    `json:"status"` // running, ok, error

	Error     string    `json:"error,omitempty"`

	Done      int64     `json:"done,omitempty"`

	Total     int64     `json:"total,omitempty"`

	UpdatedAt time.Time `json:"updatedAt"`

}



type transferActive struct {

	Name  string `json:"name"`

	Done  int64  `json:"done"`

	Total int64  `json:"total"`

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

// finishTransferLog marca a entrada «running» do mesmo nome como concluída (evita duplicados).

func (b *sshBundle) finishTransferLog(name, status, errMsg string) {

	b.transferMu.Lock()

	defer b.transferMu.Unlock()

	for i := len(b.transferLog) - 1; i >= 0; i-- {

		if b.transferLog[i].Name == name && b.transferLog[i].Status == "running" {

			b.transferLog[i].Status = status

			b.transferLog[i].Error = errMsg

			b.transferLog[i].UpdatedAt = time.Now()

			return

		}

	}

	b.transferLog = append(b.transferLog, transferRecord{

		Name: name, Status: status, Error: errMsg, UpdatedAt: time.Now(),

	})

	if len(b.transferLog) > 50 {

		b.transferLog = b.transferLog[len(b.transferLog)-50:]

	}

}



// setTransferProgress atualiza bytes da transferência em curso.

func (b *sshBundle) setTransferProgress(name string, done, total int64) {

	b.transferMu.Lock()

	defer b.transferMu.Unlock()

	b.activeXfer = &transferActive{Name: name, Done: done, Total: total}

	for i := range b.transferLog {

		if b.transferLog[i].Name == name && b.transferLog[i].Status == "running" {

			b.transferLog[i].Done = done

			b.transferLog[i].Total = total

			return

		}

	}

}



// updateTransferProgress atualiza bytes da transferência em curso.

func (b *sshBundle) updateTransferProgress(done, total int64) {

	b.transferMu.Lock()

	defer b.transferMu.Unlock()

	if b.activeXfer != nil {

		b.activeXfer.Done = done

		b.activeXfer.Total = total

	}

	for i := range b.transferLog {

		if b.transferLog[i].Status == "running" {

			b.transferLog[i].Done = done

			b.transferLog[i].Total = total

		}

	}

}



// clearTransferProgress limpa progresso ativo.

func (b *sshBundle) clearTransferProgress() {

	b.transferMu.Lock()

	b.activeXfer = nil

	b.transferMu.Unlock()

}



// transferStatus devolve fila, progresso e histórico recente.

func (b *sshBundle) transferStatus() map[string]any {

	b.transferMu.Lock()

	logCopy := append([]transferRecord(nil), b.transferLog...)

	var active any

	if b.activeXfer != nil {

		ac := *b.activeXfer

		active = ac

	}

	b.transferMu.Unlock()

	return map[string]any{

		"queued":  b.TM.Queued(),

		"running": b.TM.Running(),

		"active":  active,

		"recent":  logCopy,

	}

}

func (b *sshBundle) addOperation(msg, level string) {
	if level == "" {
		level = "info"
	}
	b.opMu.Lock()
	b.opLog = append(b.opLog, operationRecord{Time: time.Now(), Message: msg, Level: level})
	if len(b.opLog) > 200 {
		b.opLog = b.opLog[len(b.opLog)-200:]
	}
	b.opMu.Unlock()
}

func (b *sshBundle) operationHistory() []operationRecord {
	b.opMu.Lock()
	out := append([]operationRecord(nil), b.opLog...)
	b.opMu.Unlock()
	return out
}

func (b *sshBundle) clearOperationHistory() {
	b.opMu.Lock()
	b.opLog = nil
	b.opMu.Unlock()
}


