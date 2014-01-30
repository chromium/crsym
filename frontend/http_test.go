/* Copyright 2013 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package frontend

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/chromium/crsym/breakpad"
	"github.com/chromium/crsym/context"
)

type cacheTestSupplier struct {
	c chan breakpad.SupplierResponse
}

func (s *cacheTestSupplier) reset() {
	s.c = make(chan breakpad.SupplierResponse)
}

func (s *cacheTestSupplier) FilterAvailableModules(ctx context.Context, modules []breakpad.SupplierRequest) []breakpad.SupplierRequest {
	return modules
}

func (s *cacheTestSupplier) TableForModule(context.Context, breakpad.SupplierRequest) <-chan breakpad.SupplierResponse {
	return s.c
}

type cacheTestTable struct {
	ident string
}

func newTestTable(ident string) *cacheTestTable {
	return &cacheTestTable{ident: ident}
}

// breakpad.SymbolTable implementation:
func (t *cacheTestTable) ModuleName() string {
	return t.ident
}
func (t *cacheTestTable) Identifier() string {
	return t.ident
}
func (t *cacheTestTable) String() string {
	return t.ident
}
func (t *cacheTestTable) SymbolForAddress(uint64) *breakpad.Symbol {
	return nil
}

func TestGetTableCache(t *testing.T) {
	*cacheSize = 5

	// Create a new Handler. The mux is a throw-away.
	handler := RegisterHandlers(http.NewServeMux())
	supplier := new(cacheTestSupplier)
	supplier.reset()
	handler.Init(supplier)

	const kInitialName = "initial fill #%d"

	// Supply five tables to max out the cache.
	go func() {
		for i := 1; i <= *cacheSize; i++ {
			supplier.c <- breakpad.SupplierResponse{
				Table: newTestTable(fmt.Sprintf(kInitialName, i)),
			}
		}
		supplier.c <- breakpad.SupplierResponse{
			Error: errors.New("cache miss when should be cache hit"),
		}
		close(supplier.c)
	}()

	// Now receieve those five from the cache, twice.
	for iter := 0; iter < 2; iter++ {
		for i := 1; i <= *cacheSize; i++ {
			ident := fmt.Sprintf(kInitialName, i)

			table, err := handler.getTable(context.Background(), breakpad.SupplierRequest{"module", ident})
			if err != nil {
				t.Errorf("Error getting '%s': %v", ident, err)
				continue
			}

			if table.Identifier() != ident {
				t.Errorf("Identifier mismatch, expected '%s', got '%s'", ident, table.Identifier())
			}
		}
	}

	// Receive pending messages until the channel gets closed.
	for _ = range supplier.c {
	}
	supplier.reset()

	// After iterating through the cache twice, initial #5 is MRU, so #1 will
	// be the first to be evicted.
	const kEvictFirst = "evict initial fill #1"
	go func() {
		supplier.c <- breakpad.SupplierResponse{
			Table: newTestTable(kEvictFirst),
		}
		supplier.c <- breakpad.SupplierResponse{
			Error: errors.New("unexpected supplier request"),
		}
		close(supplier.c)
	}()

	// Get a different table, which will evict #1.
	table, err := handler.getTable(context.Background(), breakpad.SupplierRequest{"module", kEvictFirst})
	if err != nil {
		t.Errorf("error getting '%s': %v", kEvictFirst, err)
	} else {
		if table.Identifier() != kEvictFirst {
			t.Errorf("Identifier mismatch, expected '%s', got '%s'", kEvictFirst, table.Identifier())
		}
	}

	// Now get a table that should be in the cache.
	ident := fmt.Sprintf(kInitialName, 3)
	table, err = handler.getTable(context.Background(), breakpad.SupplierRequest{"module", ident})
	if err != nil {
		t.Errorf("error getting '%s' after evicting #1: %v", ident, err)
	} else {
		if table.Identifier() != ident {
			t.Errorf("Identifier mismatch, expected '%s', got '%s'", ident, table.Identifier())
		}
	}

	cacheOrder := []string{
		fmt.Sprintf(kInitialName, 2),
		fmt.Sprintf(kInitialName, 4),
		fmt.Sprintf(kInitialName, 5),
		kEvictFirst,
		fmt.Sprintf(kInitialName, 3),
	}
	i := 0
	for e := handler.mru.Front(); e != nil; e = e.Next() {
		ident = cacheOrder[i]
		if e.Value.(breakpad.SymbolTable).Identifier() != ident {
			t.Errorf("cache index %d mismatch, expected '%s', got '%v'", i, ident, e.Value)
		}
		if _, ok := handler.symbolCache[ident]; !ok {
			t.Errorf("cache entry '%s' not present in symbol cache", ident)
		}
		i++
	}
	if len(handler.symbolCache) != *cacheSize {
		t.Errorf("symbol cache size mismatch, expected %d, got %d", *cacheSize, len(handler.symbolCache))
	}
}
