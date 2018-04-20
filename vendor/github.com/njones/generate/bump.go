package generate

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// bumpToMode takes a version string and two compare maps to determine
// how to bump the next version and returns the type to bump
// TODO(njones): return a bumpType that has more contextual information
// about why a bump is happing so that it can be saved and displayed
// and maybe it could help with heuristics in the future.
func bumpToMode(s string, old, new map[string]signature) (rtn bumpType) {
	// the current simple rules on how to bump a version number
	// * (Function|Interface|Struct) removed from Exported list (major)
	// * parameter removed from exported function               (major)
	// * field removed from exported struct                     (major)
	// * parameter order switched in function                   (major)
	// * parameter added to function             --non-variadic (major)
	// * return added to function                               (major)
	//
	// * (Function|Interface|Struct) added to Exported list     (minor)
	// * field added to struct                                  (minor)
	// * variadic parameter added to function (TODO)            (minor)
	//
	// * all others                                             (patch)

	// check if a new exported value was added
	if len(new) > len(old) {
		rtn = bumpMinor
	}

	// check if removed
	if len(new) < len(old) {
		return bumpMajor
	}

	// check individual
	for nk, nv := range new {
		_, ok := old[nk]
		if !ok { // export removed
			return bumpMajor
		}

		// check if something has changed in the signature
		if nv.Dig != old[nk].Dig {

			switch nv.Kind {
			case "func":
				var nj = make(map[string][]string)
				var oj = make(map[string][]string)
				err := json.Unmarshal([]byte(nv.JSON), nj)
				if err != nil {
					// hummmm figure this out.... what to do with an error...
				}
				_ = json.Unmarshal([]byte(old[nk].JSON), oj)

				if len(nj) == 0 {
					return bumpMajor // something got removed...
				}
				// check if: parameter removed from exported function
			case "struct":
				// check if: field is removed from exported struct
			case "interface":
				// check if: method removed from exported interface
			}

		}
	}

	return bumpPatch
}

func bump(s string, t bumpType) (string, error) {
	ss := strings.Split(s, ".")
	if len(ss) != 3 {
		return "", fmt.Errorf("need to have three values for the version")
	}
	switch t {
	case bumpPatch:
		n, err := strconv.ParseInt(ss[2], 10, 64)
		if err != nil {
			return "", err
		}
		n++
		ss[2] = strconv.FormatInt(n, 10)
	case bumpMinor:
		n, err := strconv.ParseInt(ss[1], 10, 64)
		if err != nil {
			return "", err
		}
		n++
		ss[1] = strconv.FormatInt(n, 10)
		ss[2] = "0"
	case bumpMajor:
		n, err := strconv.ParseInt(ss[0], 10, 64)
		if err != nil {
			return "", err
		}
		n++
		ss[0] = strconv.FormatInt(n, 10)
		ss[1], ss[2] = "0", "0"
	}
	return strings.Join(ss, "."), nil
}
