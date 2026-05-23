package editor

import "gopkg.in/yaml.v3"

// yamlUnmarshal is exposed as a private function name so call sites read
// naturally without re-importing the yaml package directly in every file.
func yamlUnmarshal(in []byte, out any) error {
	return yaml.Unmarshal(in, out)
}
