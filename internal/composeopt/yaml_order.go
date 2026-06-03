package composeopt

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// NormalizeYAMLInput limpa o texto antes de analisar ou preservar ordem.
func NormalizeYAMLInput(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	if strings.HasPrefix(s, "\ufeff") {
		s = strings.TrimPrefix(s, "\ufeff")
	}
	return s
}

// MarshalPreservingOrder aplica o mapa atualizado sobre o YAML original, mantendo ordem e comentários.
func MarshalPreservingOrder(original string, updated map[string]any) (string, error) {
	original = NormalizeYAMLInput(original)
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(original), &doc); err != nil {
		return "", err
	}
	if len(doc.Content) == 0 {
		b, err := yaml.Marshal(updated)
		return string(b), err
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		b, err := yaml.Marshal(updated)
		return string(b), err
	}
	syncMapIntoNode(root, updated, "")
	if svc, _ := updated["services"].(map[string]any); svc != nil {
		if servicesNode := findMappingChild(root, "services"); servicesNode != nil {
			relocateOrphanComments(servicesNode)
		}
	}
	sanitizeYAMLTree(&doc)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", err
	}
	out := strings.TrimRight(buf.String(), "\n")
	if out != "" {
		out += "\n"
	}
	if err := validateYAML(out); err != nil {
		return "", fmt.Errorf("YAML gerado inválido: %w", err)
	}
	return out, nil
}

func validateYAML(s string) error {
	var doc yaml.Node
	return yaml.Unmarshal([]byte(s), &doc)
}

func findMappingChild(root *yaml.Node, key string) *yaml.Node {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i+1]
		}
	}
	return nil
}

// relocateOrphanComments move comentários Foot presos no fim de um serviço para o próximo bloco.
func relocateOrphanComments(services *yaml.Node) {
	if services == nil || services.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(services.Content); i += 2 {
		orphan := drainServiceFootComments(services.Content[i+1])
		if orphan == "" {
			continue
		}
		if i+2 < len(services.Content) {
			nextKey := services.Content[i+2]
			if nextKey.HeadComment != "" {
				nextKey.HeadComment = strings.TrimSpace(orphan + "\n" + nextKey.HeadComment)
			} else {
				nextKey.HeadComment = strings.TrimSpace(orphan)
			}
		} else {
			services.Content[i].FootComment = strings.TrimSpace(orphan)
		}
	}
}

func drainServiceFootComments(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	if n.FootComment != "" && strings.Contains(n.FootComment, "#") {
		fc := strings.TrimSpace(n.FootComment)
		n.FootComment = ""
		return fc
	}
	for _, c := range n.Content {
		if fc := drainServiceFootComments(c); fc != "" {
			return fc
		}
	}
	return ""
}

func sanitizeYAMLTree(n *yaml.Node) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			sanitizeYAMLTree(c)
		}
	case yaml.MappingNode:
		sanitizeMappingContent(n)
		for i := 1; i < len(n.Content); i += 2 {
			sanitizeYAMLTree(n.Content[i])
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			sanitizeYAMLTree(c)
		}
	default:
		n.Value = strings.ReplaceAll(n.Value, "\u00a0", " ")
	}
}

func sanitizeMappingContent(n *yaml.Node) {
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	var clean []*yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		keyN := n.Content[i]
		valN := n.Content[i+1]
		key := strings.TrimSpace(keyN.Value)
		if key == "" || strings.HasPrefix(key, "#") || !isValidYAMLKey(key) {
			continue
		}
		clean = append(clean, keyN, valN)
	}
	n.Content = clean
}

func isValidYAMLKey(key string) bool {
	for _, r := range key {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func insertAfterKey(n *yaml.Node, key string, valNode *yaml.Node, afterKey string) {
	if n.Kind != yaml.MappingNode {
		return
	}
	insertIdx := len(n.Content)
	if afterKey != "" {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == afterKey {
				insertIdx = i + 2
				// Comentários no valor anterior (FootComment) pertencem ao YAML seguinte — não ficam entre chaves novas.
				if val := n.Content[i+1]; val != nil {
					val.FootComment = ""
					if strings.HasPrefix(strings.TrimSpace(val.LineComment), "#") {
						val.LineComment = ""
					}
				}
				break
			}
		}
	}
	keyN := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	newContent := make([]*yaml.Node, 0, len(n.Content)+2)
	newContent = append(newContent, n.Content[:insertIdx]...)
	newContent = append(newContent, keyN, valNode)
	newContent = append(newContent, n.Content[insertIdx:]...)
	n.Content = newContent
}

func mapKeyInsertAfter(parentPath, newKey string) string {
	switch newKey {
	case "cpus":
		return "memory"
	case "restart_policy":
		if parentPath == "deploy" {
			return "resources"
		}
		return ""
	case "update_config":
		return "restart_policy"
	case "shm_size":
		return ""
	default:
		return ""
	}
}

func syncMapIntoNode(n *yaml.Node, data map[string]any, parentKey string) {
	if n == nil || data == nil || n.Kind != yaml.MappingNode {
		return
	}
	seen := make(map[string]bool, len(data))
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i].Value
		seen[key] = true
		child, ok := data[key]
		if !ok {
			continue
		}
		syncValueIntoNode(n.Content[i+1], child, key)
	}
	var pending []string
	for key := range data {
		if !seen[key] {
			pending = append(pending, key)
		}
	}
	for _, key := range sortedMapKeysFromList(pending) {
		valN := &yaml.Node{}
		setNodeFromValue(valN, data[key])
		after := mapKeyInsertAfter(parentKey, key)
		insertAfterKey(n, key, valN, after)
	}
}

func sortedMapKeysFromList(keys []string) []string {
	m := make(map[string]any, len(keys))
	for _, k := range keys {
		m[k] = nil
	}
	return sortedMapKeys(m)
}

func syncValueIntoNode(n *yaml.Node, v any, parentKey string) {
	switch val := v.(type) {
	case map[string]any:
		if n.Kind != yaml.MappingNode {
			n.Kind = yaml.MappingNode
			n.Tag = "!!map"
			n.Content = nil
		}
		if nodeDataEqual(n, val) {
			return
		}
		syncMapIntoNode(n, val, parentKey)
	case []any:
		if n.Kind != yaml.SequenceNode {
			n.Kind = yaml.SequenceNode
			n.Tag = "!!seq"
			n.Content = nil
			for _, item := range val {
				itemN := &yaml.Node{}
				setNodeFromValue(itemN, item)
				n.Content = append(n.Content, itemN)
			}
			return
		}
		if nodeDataEqual(n, val) {
			return
		}
		syncSequenceIntoNode(n, val)
	default:
		if n != nil && n.Kind == yaml.ScalarNode && nodeDataEqual(n, v) {
			return
		}
		setScalar(n, v)
	}
}

func syncSequenceIntoNode(n *yaml.Node, val []any) {
	for i := 0; i < len(n.Content) && i < len(val); i++ {
		syncValueIntoNode(n.Content[i], val[i], "")
	}
	for i := len(n.Content); i < len(val); i++ {
		itemN := &yaml.Node{}
		setNodeFromValue(itemN, val[i])
		n.Content = append(n.Content, itemN)
	}
	// Só trunca entradas extra se forem nós YAML válidos (evita apagar estrutura atípica).
	for len(n.Content) > len(val) && len(val) > 0 {
		last := len(n.Content) - 1
		if n.Content[last] == nil || n.Content[last].Kind == yaml.AliasNode {
			break
		}
		n.Content = n.Content[:len(val)]
	}
}

func nodeDataEqual(n *yaml.Node, data any) bool {
	if n == nil {
		return data == nil
	}
	var cur any
	if err := n.Decode(&cur); err != nil {
		return false
	}
	return yamlDataEqual(cur, data)
}

func yamlDataEqual(a, b any) bool {
	return reflect.DeepEqual(normalizeYAMLData(a), normalizeYAMLData(b))
}

func normalizeYAMLData(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, c := range t {
			out[k] = normalizeYAMLData(c)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, c := range t {
			out[fmt.Sprint(k)] = normalizeYAMLData(c)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, c := range t {
			out[i] = normalizeYAMLData(c)
		}
		return out
	case string:
		return strings.TrimSpace(strings.ReplaceAll(t, "\u00a0", " "))
	case float64:
		if t == float64(int64(t)) {
			return int64(t)
		}
		return t
	default:
		return v
	}
}

func setNodeFromValue(n *yaml.Node, v any) {
	switch val := v.(type) {
	case map[string]any:
		n.Kind = yaml.MappingNode
		n.Tag = "!!map"
		n.Content = nil
		keys := sortedMapKeys(val)
		for _, key := range keys {
			keyN := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
			valN := &yaml.Node{}
			setNodeFromValue(valN, val[key])
			n.Content = append(n.Content, keyN, valN)
		}
	case []any:
		n.Kind = yaml.SequenceNode
		n.Tag = "!!seq"
		n.Content = nil
		for _, item := range val {
			itemN := &yaml.Node{}
			setNodeFromValue(itemN, item)
			n.Content = append(n.Content, itemN)
		}
	default:
		setScalar(n, v)
	}
}

// sortedMapKeys ordem estável para nós novos (cpus após memory quando possível).
func sortedMapKeys(m map[string]any) []string {
	priority := []string{
		"services", "networks", "volumes", "image", "networks", "ports", "environment",
		"command", "volumes", "deploy", "mode", "endpoint_mode", "placement",
		"resources", "limits", "reservations", "memory", "cpus", "restart_policy", "update_config",
		"parallelism", "delay", "failure_action", "order", "condition", "max_attempts",
	}
	seen := make(map[string]bool, len(m))
	var keys []string
	for _, p := range priority {
		if _, ok := m[p]; ok && !seen[p] {
			keys = append(keys, p)
			seen[p] = true
		}
	}
	rest := make([]string, 0, len(m))
	for k := range m {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	for i := 0; i < len(rest); i++ {
		for j := i + 1; j < len(rest); j++ {
			if rest[j] < rest[i] {
				rest[i], rest[j] = rest[j], rest[i]
			}
		}
	}
	return append(keys, rest...)
}

func setScalar(n *yaml.Node, v any) {
	n.Kind = yaml.ScalarNode
	switch t := v.(type) {
	case string:
		n.Value = strings.ReplaceAll(t, "\u00a0", " ")
		n.Tag = "!!str"
	case bool:
		if t {
			n.Value = "true"
		} else {
			n.Value = "false"
		}
		n.Tag = "!!bool"
	case int:
		n.Value = fmt.Sprint(t)
		n.Tag = "!!int"
	case int64:
		n.Value = fmt.Sprint(t)
		n.Tag = "!!int"
	case float64:
		n.Value = fmt.Sprint(t)
		n.Tag = "!!float"
	default:
		n.Value = fmt.Sprint(v)
		n.Tag = "!!str"
	}
}
