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

/*
	Package frontend provides a HTTP server that accepts input for symbolization
	in various formats and returns the symbolized output.
*/
package frontend

import (
	"bytes"
	"container/list"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path"
	"sync"

	"flag"
	"github.com/chromium/crsym/breakpad"
	log "github.com/golang/glog"
)

var (
	// Path to the static files directory for the frontend.
	frontendFiles string

	cacheSize = flag.Int("symbol_cache_size", 30, "Number of symbol files to keep in an MRU cache")

	// Extra data to put on the homepage.
	statusData []template.HTML
)

// SetFilesPath sets the path to where the static frontend files reside on disk.
func SetFilesPath(p string) {
	frontendFiles = p
}

// SetHomePageStatus adds extra strings to the top-right corner of the main UI.
func SetHomePageStatus(status []string) {
	statusData = make([]template.HTML, len(status))
	for i, s := range status {
		statusData[i] = template.HTML(s)
	}
}

// RegisterHandlers adds the frontend endpoints to the provided ServeMux and
// returns the Handler state. SetFilesPath should be called before this.
func RegisterHandlers(mux *http.ServeMux) *Handler {
	mux.HandleFunc("/", indexHandler)

	staticDir := "/static/"
	staticHandler := http.FileServer(http.Dir(frontendFiles))
	mux.Handle(staticDir, http.StripPrefix("/static", staticHandler))

	handler := &Handler{
		mu:          new(sync.Mutex),
		mru:         list.New(),
		symbolCache: make(map[string]*list.Element),
	}
	// Initialize the cache with an empty list of size |cacheSize|.
	for i := 0; i < *cacheSize; i++ {
		handler.mru.PushBack(nil)
	}
	mux.Handle("/_/service", handler)

	return handler
}

func indexHandler(rw http.ResponseWriter, req *http.Request) {
	tpl, err := template.ParseFiles(path.Join(frontendFiles, "home.html"))
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(rw, err)
		return
	}

	rw.Header().Set("Content-type", "text/html")
	tpl.Execute(rw, struct {
		StatusData []template.HTML
	}{
		statusData,
	})
}

// Type Handler stores the breakpad.Supplier and other server state.
type Handler struct {
	supplier          breakpad.Supplier
	frameService      breakpad.AnnotatedFrameService
	moduleInfoService breakpad.ModuleInfoService

	// mu is the mutex that protects the two objects below.
	mu *sync.Mutex
	// mru contains a list of SymbolTable objects most recently fetched from the
	// supplier, with newest at the end.
	mru *list.List
	// symbolCache maps SymbolTable.Identifier() to elements in |mru| for fast
	// cache lookup.
	symbolCache map[string]*list.Element
}

// Init sets the breakpad supplier to use. This should be called before starting
// the server.
func (h *Handler) Init(supplier breakpad.Supplier) {
	h.supplier = supplier
}

// SetAnnotatedFrameService sets the backend implementation that fetches crash
// report frame information. If nil, the CrashKeyParser cannot be used.
func (h *Handler) SetAnnotatedFrameService(s breakpad.AnnotatedFrameService) {
	h.frameService = s
}

// SetModuleInfoService sets the backend for querying for module information.
// If nil, the module_info input type cannot be used.
func (h *Handler) SetModuleInfoService(s breakpad.ModuleInfoService) {
	h.moduleInfoService = s
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	logRequest(req)

	if req.Method != "POST" {
		replyError(req, rw, http.StatusMethodNotAllowed, "Only POSTs allowed")
		return
	}

	input := req.FormValue("input")
	inputRequired := true

	var parser InputParser
	switch req.FormValue("input_type") {
	case "fragment":
		parser = h.handleFragment(rw, req)
	case "apple":
		parser = new(AppleInputParser)
	case "stackwalk":
		parser = NewStackwalkInputParser()
	case "crash_key":
		parser = h.handleCrashKey(rw, req)
		inputRequired = false
	case "module_info":
		parser = h.handleModuleInfo(rw, req)
		inputRequired = false
	case "android":
		parser = h.handleAndroid(rw, req)
	default:
		replyError(req, rw, http.StatusNotImplemented, "Unknown input_type")
	}

	if parser == nil {
		return
	}
	if input == "" && inputRequired {
		replyError(req, rw, http.StatusBadRequest, "Missing input")
		return
	}

	if err := parser.ParseInput(input); err != nil {
		replyError(req, rw, http.StatusBadRequest, err.Error())
		return
	}

	requiredModules := parser.RequiredModules()
	if parser.FilterModules() {
		requiredModules = h.supplier.FilterAvailableModules(requiredModules)
	}

	var tables []breakpad.SymbolTable
	for _, moduleRequest := range requiredModules {
		table, err := h.getTable(moduleRequest)
		if err != nil {
			replyError(req, rw, 404, err.Error())
			return
		}
		tables = append(tables, table)
	}

	output := parser.Symbolize(tables)
	io.WriteString(rw, output)
}

// getTable looks up the requested module in the server cache and returns it
// if present. If it is not, this performs a blocking call to the Supplier and
// caches the result.
func (h *Handler) getTable(request breakpad.SupplierRequest) (breakpad.SymbolTable, error) {
	table := h.loadCachedTable(request)
	if table != nil {
		return table, nil
	}

	// Not cached, so fetch it from the supplier.
	resp := <-h.supplier.TableForModule(request)
	if resp.Error != nil {
		return nil, resp.Error
	}

	// Take the LRU item from the cache and remove it.
	h.mu.Lock()
	defer h.mu.Unlock()
	elm := h.mru.Front()
	if elm.Value != nil {
		delete(h.symbolCache, elm.Value.(breakpad.SymbolTable).Identifier())
	}

	// Insert the new table as the MRU one.
	ident := resp.Table.Identifier()
	elm.Value = resp.Table
	h.symbolCache[ident] = elm

	h.mru.MoveToBack(elm)

	return resp.Table, nil
}

// loadCachedTable looks in the cache for the requested symbol table, marks it
// as recently used if found, and returns it. Returns nil for no cache entry.
func (h *Handler) loadCachedTable(request breakpad.SupplierRequest) breakpad.SymbolTable {
	h.mu.Lock()
	defer h.mu.Unlock()

	if elm, ok := h.symbolCache[request.Identifier]; ok {
		h.mru.MoveToBack(elm)
		return elm.Value.(breakpad.SymbolTable)
	}
	return nil
}

// handleFragment extracts fragment-specific input from the HTTP request and
// returns a FragmentInputParser if successful.
func (h *Handler) handleFragment(rw http.ResponseWriter, req *http.Request) InputParser {
	module := req.FormValue("module")
	ident := req.FormValue("ident")
	if module == "" || ident == "" {
		replyError(req, rw, http.StatusBadRequest, "Missing module or ident")
		return nil
	}

	loadAddress, err := breakpad.ParseAddress(req.FormValue("load_address"))
	if err != nil {
		replyError(req, rw, http.StatusBadRequest, fmt.Sprintf("Load address: %s", err))
		return nil
	}

	return NewFragmentInputParser(module, ident, loadAddress)
}

// handleCrashKey extracts the crash-key-specific input and returns an input
// parser if successful.
func (h *Handler) handleCrashKey(rw http.ResponseWriter, req *http.Request) InputParser {
	reportID := req.FormValue("report_id")
	key := req.FormValue("crash_key")
	if reportID == "" || key == "" {
		replyError(req, rw, http.StatusBadRequest, "Missing report ID or crash key")
		return nil
	}

	return NewCrashKeyInputParser(h.frameService, reportID, key)
}

// handleModuleInfo just looks up the module information for a product and version.
func (h *Handler) handleModuleInfo(rw http.ResponseWriter, req *http.Request) InputParser {
	product := req.FormValue("product_name")
	version := req.FormValue("product_version")
	if product == "" || version == "" {
		replyError(req, rw, http.StatusBadRequest, "Missing product name or version")
		return nil
	}

	return NewModuleInfoInputParser(h.moduleInfoService, product, version)
}

// handleAndroid parses a debug log (logcat) and outputs the stack.  Version number
// of the android chrome build is an optional input.
func (h *Handler) handleAndroid(rw http.ResponseWriter, req *http.Request) InputParser {
	version := req.FormValue("android_chrome_version")
	return NewAndroidInputParser(h.moduleInfoService, version)
}

func replyError(req *http.Request, rw http.ResponseWriter, code int, message string) {
	log.Infof("ERROR reply for %s, code %d (%q)", getUserIp(req), code, message)
	rw.WriteHeader(code)
	io.WriteString(rw, message)
}

// CacheStatus returns a HTML fragment that displays the current status of the
// symbol cache.
func (h *Handler) CacheStatus() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	data := struct {
		NumEntries, CacheSize int
		Cache                 []string
	}{
		NumEntries: len(h.symbolCache),
		CacheSize:  *cacheSize,
		Cache:      make([]string, 0),
	}

	for e := h.mru.Front(); e != nil; e = e.Next() {
		v := "<nil>"
		if e.Value != nil {
			v = e.Value.(breakpad.SymbolTable).String()
		}
		data.Cache = append(data.Cache, v)
	}

	buf := bytes.NewBuffer(nil)
	if err := cacheStatusTemplate.Execute(buf, data); err != nil {
		return fmt.Sprintf("Error: %s", err.Error())
	}
	return buf.String()
}

func getUserIp(req *http.Request) string {
	ip := req.Header.Get("X-Proxied-User-Ip")
	if ip == "" {
		ip = req.Header.Get("X-Forwarded-For")
	}
	if ip == "" {
		ip = req.RemoteAddr
	}
	return ip
}

func logRequest(req *http.Request) {
	log.Infof("REQUEST to symbolize input type %q from %s", req.FormValue("input_type"), getUserIp(req))
}

var cacheStatusTemplate = template.Must(template.New("cache").Parse(
	`<div style="font-weight:bold">
	Capacity: {{.NumEntries}} / {{.CacheSize}}
</div>
<ol start="0">
	{{range .Cache}}
	<li>{{.}}</li>
	{{end}}
</ol>`))
