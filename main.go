package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"git.meow.tf/ow-api/ow-api/cache"
	"git.meow.tf/ow-api/ow-api/json-patch"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/cors"
	"log"
	"net/http"
	"s32x.com/ovrstat/ovrstat"
	"strings"
	"time"
)

const (
	Version = "2.1.0"

	OpAdd    = "add"
	OpRemove = "remove"
)

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

type gamesStats struct {
	Played int64 `json:"played"`
	Won    int64 `json:"won"`
}

type awardsStats struct {
	Cards  int64 `json:"cards"`
	Medals int64 `json:"medals"`
	Bronze int64 `json:"medalsBronze"`
	Silver int64 `json:"medalsSilver"`
	Gold   int64 `json:"medalsGold"`
}

var (
	flagBind      = flag.String("bind-address", ":8080", "Address to bind to for http requests")
	flagCache     = flag.String("cache", "redis://localhost:6379", "Cache uri or 'none' to disable")
	flagCacheTime = flag.Int("cacheTime", 600, "Cache time in seconds")

	cacheProvider cache.Provider

	cacheTime time.Duration

	profilePatch *jsonpatch.Patch

	heroNames []string
)

func main() {
	loadHeroNames()

	cacheProvider = cache.ForURI(*flagCache)

	cacheTime = time.Duration(*flagCacheTime) * time.Second

	var err error

	ops := []patchOperation{
		{Op: OpRemove, Path: "/quickPlayStats/topHeroes"},
		{Op: OpRemove, Path: "/competitiveStats/topHeroes"},
		{Op: OpRemove, Path: "/quickPlayStats/careerStats"},
		{Op: OpRemove, Path: "/competitiveStats/careerStats"},
	}

	profilePatch, err = patchFromOperations(ops)

	if err != nil {
		log.Fatalln("Unable to create base patch:", err)
	}

	router := httprouter.New()

	// PC
	router.GET("/v1/stats/pc/:region/:tag/heroes/:heroes", injectPlatform("pc", heroes))
	router.GET("/v1/stats/pc/:region/:tag/profile", injectPlatform("pc", profile))
	router.GET("/v1/stats/pc/:region/:tag/complete", injectPlatform("pc", stats))

	// Console
	router.GET("/v1/stats/psn/:tag/heroes/:heroes", injectPlatform("psn", heroes))
	router.GET("/v1/stats/psn/:tag/profile", injectPlatform("psn", profile))
	router.GET("/v1/stats/psn/:tag/complete", injectPlatform("psn", stats))
	router.GET("/v1/stats/xbl/:tag/heroes/:heroes", injectPlatform("xbl", heroes))
	router.GET("/v1/stats/xbl/:tag/profile", injectPlatform("xbl", profile))
	router.GET("/v1/stats/xbl/:tag/complete", injectPlatform("xbl", stats))

	// Version
	router.GET("/v1/version", versionHandler)

	router.HEAD("/v1/status", statusHandler)
	router.GET("/v1/status", statusHandler)

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
	})

	router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "OPTIONS" {
			http.NotFound(w, r)
		}
	})

	log.Fatal(http.ListenAndServe(*flagBind, c.Handler(router)))
}

func loadHeroNames() {
	stats, err := ovrstat.PCStats("cats-11481")

	if err != nil {
		return
	}

	m := make(map[string]bool)

	for k := range stats.QuickPlayStats.TopHeroes {
		m[k] = true
	}

	for k := range stats.QuickPlayStats.CareerStats {
		m[k] = true
	}

	heroNames = make([]string, 0)

	for k := range m {
		heroNames = append(heroNames, k)
	}
}

func injectPlatform(platform string, handler httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ps = append(ps, httprouter.Param{Key: "platform", Value: platform})

		handler(w, r, ps)
	}
}

func statsResponse(w http.ResponseWriter, ps httprouter.Params, patch *jsonpatch.Patch, cacheAddition string) ([]byte, error) {
	var stats *ovrstat.PlayerStats
	var err error

	tag := ps.ByName("tag")

	tag = strings.Replace(tag, "#", "-", -1)

	cacheKey := generateCacheKey(ps)

	if cacheAddition != "" {
		cacheKey += "-" + cacheAddition
	}

	if region := ps.ByName("region"); region != "" {
		stats, err = ovrstat.PCStats(tag)
	} else if platform := ps.ByName("platform"); platform != "" {
		stats, err = ovrstat.ConsoleStats(platform, tag)
	} else {
		return nil, errors.New("unknown region/platform")
	}

	if err != nil {
		return nil, err
	}

	// Caching of full response for modification

	res, err := cacheProvider.Get(cacheKey)

	if res != nil && err == nil {
		if patch != nil {
			res, err = patch.Apply(res)
		}

		return res, err
	}

	extra := make([]patchOperation, 0)

	if hs, ok := stats.QuickPlayStats.CareerStats["allHeroes"]; ok {
		games := &gamesStats{}

		games.Played = valueOrDefault(hs.Game, "gamesPlayed", 0)
		games.Won = valueOrDefault(hs.Game, "gamesWon", 0)

		awards := &awardsStats{}

		awards.Cards = valueOrDefault(hs.MatchAwards, "cards", 0)
		awards.Medals = valueOrDefault(hs.MatchAwards, "medals", 0)
		awards.Bronze = valueOrDefault(hs.MatchAwards, "medalsBronze", 0)
		awards.Silver = valueOrDefault(hs.MatchAwards, "medalsSilver", 0)
		awards.Gold = valueOrDefault(hs.MatchAwards, "medalsGold", 0)

		extra = append(extra, patchOperation{
			Op:    OpAdd,
			Path:  "/quickPlayStats/games",
			Value: games,
		}, patchOperation{
			Op:    OpAdd,
			Path:  "/quickPlayStats/awards",
			Value: awards,
		})
	}

	if hs, ok := stats.CompetitiveStats.CareerStats["allHeroes"]; ok {
		games := &gamesStats{}

		games.Played = valueOrDefault(hs.Game, "gamesPlayed", 0)
		games.Won = valueOrDefault(hs.Game, "gamesWon", 0)

		awards := &awardsStats{}

		awards.Cards = valueOrDefault(hs.MatchAwards, "cards", 0)
		awards.Medals = valueOrDefault(hs.MatchAwards, "medals", 0)
		awards.Bronze = valueOrDefault(hs.MatchAwards, "medalsBronze", 0)
		awards.Silver = valueOrDefault(hs.MatchAwards, "medalsSilver", 0)
		awards.Gold = valueOrDefault(hs.MatchAwards, "medalsGold", 0)

		extra = append(extra, patchOperation{
			Op:    OpAdd,
			Path:  "/competitiveStats/games",
			Value: games,
		}, patchOperation{
			Op:    OpAdd,
			Path:  "/competitiveStats/awards",
			Value: awards,
		})
	}

	b, err := json.Marshal(stats)

	if err != nil {
		return nil, err
	}

	if len(extra) > 0 {
		extraPatch, err := patchFromOperations(extra)

		if err != nil {
			return nil, err
		}

		b, err = extraPatch.Apply(b)

		if err != nil {
			return nil, err
		}
	}

	// Cache response
	if cacheTime > 0 {
		cacheProvider.Set(cacheKey, b, cacheTime)
	}

	if patch != nil {
		// Apply filter patch
		b, err = patch.Apply(b)
	}

	return b, err
}

func stats(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	data, err := statsResponse(w, ps, nil, "")

	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.Write(data)
}

func profile(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// Cache result for profile specifically
	data, err := statsResponse(w, ps, profilePatch, "profile")

	if err != nil {
		writeError(w, err)
		return
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

	cacheKey := md5.New().Sum([]byte(strings.Join(names, ",")))

	// Create a patch to remove all but specified heroes
	data, err := statsResponse(w, ps, patch, "heroes-"+hex.EncodeToString(cacheKey))

	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.Write(data)
}

func valueOrDefault(m map[string]interface{}, key string, d int64) int64 {
	if v, ok := m[key]; ok {
		switch v.(type) {
		case int64:
			return v.(int64)
		case int:
			return int64(v.(int))
		}
	}
	return d
}

func patchFromOperations(ops []patchOperation) (*jsonpatch.Patch, error) {
	patchBytes, err := json.Marshal(ops)

	if err != nil {
		return nil, err
	}

	patch, err := jsonpatch.DecodePatch(patchBytes)

	if err != nil {
		return nil, err
	}

	return &patch, nil
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

type errorObject struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")

	if err == ovrstat.ErrPlayerNotFound {
		w.WriteHeader(http.StatusNotFound)
	}

	if err := json.NewEncoder(w).Encode(&errorObject{Error: err.Error()}); err != nil {
		return
	}
}

func generateCacheKey(ps httprouter.Params) string {
	var cacheKey string

	tag := ps.ByName("tag")

	if region := ps.ByName("region"); region != "" {
		cacheKey = "pc-" + tag
	} else if platform := ps.ByName("platform"); platform != "" {
		cacheKey = platform + "-" + tag
	}

	return cacheKey
}
