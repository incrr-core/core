package generate

import (
	"encoding/hex"
	"hash"
	"os/exec"
)

func hashToStr(h hash.Hash, s string) string {
	h.Write([]byte(s))
	hash := h.Sum(nil)
	h.Reset()
	return hex.EncodeToString(hash[:])
}

func checkExtHash(old, new string) (string, bool) { return new, old == new }

func DoGitHash() (string, error) {
	b, err := exec.Command("git", "rev-parse", "HEAD").Output()
	return string(b), err
}
