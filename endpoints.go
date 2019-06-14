package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/julienschmidt/httprouter"
	"net/http"
	"sort"
	"strings"
)

func stats(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	data, err := statsResponse(w, ps, nil)

	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.Write(data)
}

func profile(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	cacheKey := generateCacheKey(ps) + "-profile"

	res, err := cacheProvider.Get(cacheKey)

	if res != nil && err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write(res)
		return
	}

	// Cache result for profile specifically
	data, err := statsResponse(w, ps, profilePatch)

	if err != nil {
		writeError(w, err)
		return
	}

	if cacheTime > 0 {
		cacheProvider.Set(cacheKey, data, cacheTime)
	}

	w.Header().Set("Content-Type", "application/json")

	w.Write(data)
}

func heroes(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	names := strings.Split(ps.ByName("heroes"), ",")

	if len(names) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, errors.New("name list must contain at least one hero"))
		return
	}

	sort.Strings(names)

	cacheKey := generateCacheKey(ps) + "-heroes-" + hex.EncodeToString(md5.New().Sum([]byte(strings.Join(names, ","))))

	res, err := cacheProvider.Get(cacheKey)

	if res != nil && err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write(res)
		return
	}

	nameMap := make(map[string]bool)

	for _, name := range names {
		nameMap[name] = true
	}

	ops := make([]patchOperation, 0)

	for _, heroName := range heroNames {
		if _, exists := nameMap[heroName]; !exists {
			ops = append(ops, patchOperation{
				Op:   OpRemove,
				Path: "/quickPlayStats/topHeroes/" + heroName,
			}, patchOperation{
				Op:   OpRemove,
				Path: "/quickPlayStats/careerStats/" + heroName,
			}, patchOperation{
				Op:   OpRemove,
				Path: "/competitiveStats/topHeroes/" + heroName,
			}, patchOperation{
				Op:   OpRemove,
				Path: "/competitiveStats/careerStats/" + heroName,
			})
		}
	}

	patch, err := patchFromOperations(ops)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, err)
		return
	}

	// Create a patch to remove all but specified heroes
	data, err := statsResponse(w, ps, patch)

	if err != nil {
		writeError(w, err)
		return
	}

	if cacheTime > 0 {
		cacheProvider.Set(cacheKey, data, cacheTime)
	}

	w.Header().Set("Content-Type", "application/json")

	w.Write(data)
}

type versionObject struct {
	Version string `json:"version"`
}

func versionHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(&versionObject{Version: Version}); err != nil {
		writeError(w, err)
	}
}

type statusObject struct {
	ResponseCode int    `json:"responseCode"`
	Error        string `json:"error,omitempty"`
}

func statusHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	w.Header().Set("Content-Type", "application/json")

	status := &statusObject{}

	res, err := http.DefaultClient.Head("https://playoverwatch.com")

	if err == nil {
		status.ResponseCode = res.StatusCode

		if res.StatusCode != http.StatusOK {
			w.WriteHeader(res.StatusCode)
		}
	} else {
		status.Error = err.Error()
	}

	if r.Method != http.MethodHead {
		if err := json.NewEncoder(w).Encode(status); err != nil {
			writeError(w, err)
		}
	}
}
