// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package mock

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/hashicorp/nomad/nomad/structs"
)

type MockVariables map[string]*structs.VariableDecrypted

func Variable() *structs.VariableDecrypted {
	return &structs.VariableDecrypted{
		VariableMetadata: structs.VariableMetadata{
			Namespace: "default",
			Path:      "/example/path",
		},
		Items: structs.VariableItems{
			"key1": "value1",
			"key2": "value2",
		},
	}
}

// Variables returns a random number of variables between min
// and max inclusive.
func Variables(minU, maxU uint8) MockVariables {
	// the unsignedness of the args is to prevent goofy parameters, they're
	// easier to work with as ints in this code.
	min := int(minU)
	max := int(maxU)
	vc := min
	// handle cases with irrational arguments. Max < Min = min
	if max > min {
		vc = rand.Intn(max-min) + min
	}
	svs := make([]*structs.VariableDecrypted, vc)
	paths := make(map[string]*structs.VariableDecrypted, vc)
	for i := 0; i < vc; i++ {
		nv := Variable()
		nv.Path = fmt.Sprintf("%s/%d", nv.Path, i)
		paths[nv.Path] = nv
		svs[i] = nv
	}
	return paths
}

func (svs MockVariables) ListPaths() []string {
	out := make([]string, 0, len(svs))
	for _, sv := range svs {
		out = append(out, sv.Path)
	}
	sort.Strings(out)
	return out
}

func (svs MockVariables) List() []*structs.VariableDecrypted {
	out := make([]*structs.VariableDecrypted, 0, len(svs))
	for _, p := range svs.ListPaths() {
		pc := svs[p].Copy()
		out = append(out, &pc)
	}
	return out
}

type MockVariablesEncrypted map[string]*structs.VariableEncrypted

func VariableEncrypted() *structs.VariableEncrypted {
	return &structs.VariableEncrypted{
		VariableMetadata: structs.VariableMetadata{
			Namespace: "default",
			Path:      "/example/path",
		},
		VariableData: structs.VariableData{
			KeyID: "foo",
			Data:  []byte("foo"),
		},
	}
}

// VariablesEncrypted returns a random number of variables between min
// and max inclusive.
func VariablesEncrypted(minU, maxU uint8) MockVariablesEncrypted {
	// the unsignedness of the args is to prevent goofy parameters, they're
	// easier to work with as ints in this code.
	min := int(minU)
	max := int(maxU)
	vc := min
	// handle cases with irrational arguments. Max < Min = min
	if max > min {
		vc = rand.Intn(max-min) + min
	}
	svs := make([]*structs.VariableEncrypted, vc)
	paths := make(map[string]*structs.VariableEncrypted, vc)
	for i := 0; i < vc; i++ {
		nv := VariableEncrypted()
		// There is an extremely rare chance of path collision because the mock
		// variables generate their paths randomly. This check will add
		// an extra component on conflict to (ideally) disambiguate them.
		if _, found := paths[nv.Path]; found {
			nv.Path = nv.Path + "/" + fmt.Sprint(time.Now().UnixNano())
		}
		paths[nv.Path] = nv
		svs[i] = nv
	}
	return paths
}

func (svs MockVariablesEncrypted) ListPaths() []string {
	out := make([]string, 0, len(svs))
	for _, sv := range svs {
		out = append(out, sv.Path)
	}
	sort.Strings(out)
	return out
}

func (svs MockVariablesEncrypted) List() []*structs.VariableEncrypted {
	out := make([]*structs.VariableEncrypted, 0, len(svs))
	for _, p := range svs.ListPaths() {
		pc := svs[p].Copy()
		out = append(out, &pc)
	}
	return out
}
