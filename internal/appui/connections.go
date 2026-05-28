package appui

import "containerway/internal/connectcfg"

type savedConnection = connectcfg.SavedConnection

// loadSavedConnections executa parte da logica deste modulo.
func loadSavedConnections() ([]savedConnection, error) {
	return connectcfg.LoadAll()
}

// saveConnections executa parte da logica deste modulo.
func saveConnections(list []savedConnection) error {
	return connectcfg.Save(list)
}

// upsertConnection executa parte da logica deste modulo.
func upsertConnection(list []savedConnection, c savedConnection) []savedConnection {
	return connectcfg.Upsert(list, c)
}

// removeConnectionByName executa parte da logica deste modulo.
func removeConnectionByName(list []savedConnection, name string) []savedConnection {
	return connectcfg.RemoveByName(list, name)
}

// findConnectionByName executa parte da logica deste modulo.
func findConnectionByName(list []savedConnection, name string) (savedConnection, bool) {
	return connectcfg.FindByName(list, name)
}
