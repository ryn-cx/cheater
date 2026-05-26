package placeholder

import "regexp"

var re = regexp.MustCompile(`<([A-Za-z_][A-Za-z0-9_]*)>`)

func Extract(command string) []string {
	matches := re.FindAllStringSubmatch(command, -1)
	seen := map[string]bool{}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func Assemble(command string, values map[string]string) string {
	return re.ReplaceAllStringFunc(command, func(match string) string {
		name := match[1 : len(match)-1]
		return values[name]
	})
}
