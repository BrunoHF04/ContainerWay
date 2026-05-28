package fyneprefs

import "encoding/json"

const (
	LeftFavoritesKey  = "explorer.left.favorites"
	RightFavoritesKey = "explorer.right.favorites"
)

// GetStringSlice lê lista de strings de preferences.json.
func GetStringSlice(key string) ([]string, error) {
	m, err := loadMap()
	if err != nil {
		return nil, err
	}
	raw, ok := m[key]
	if !ok || len(raw) == 0 {
		return []string{}, nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return []string{}, nil
	}
	return out, nil
}

// SetStringSlice grava lista de strings em preferences.json.
func SetStringSlice(key string, paths []string) error {
	m, err := loadMap()
	if err != nil {
		return err
	}
	if paths == nil {
		paths = []string{}
	}
	b, _ := json.Marshal(paths)
	m[key] = b
	return saveMap(m)
}
