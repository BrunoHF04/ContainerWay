package webapp

// stopAutomationEngine para o motor da sessão SSH web.
func (b *sshBundle) stopAutomationEngine() {
	if b == nil {
		return
	}
	b.autoEngine.Stop()
}

// startAutomationEngine inicia o motor para a sessão web.
func (b *sshBundle) startAutomationEngine(host string) error {
	path, err := automationConfigPath(host)
	if err != nil {
		return err
	}
	histPath, err := automationHistoryPath(host)
	if err != nil {
		return err
	}
	if b.Sess == nil || b.Sess.Docker == nil {
		return errDockerUnavailable
	}
	b.autoEngine.Start(b.Sess.Docker, path, histPath, nil)
	return nil
}

// automationEngineRunning indica se o motor está ativo.
func (b *sshBundle) automationEngineRunning() bool {
	if b == nil {
		return false
	}
	return b.autoEngine.Running()
}
