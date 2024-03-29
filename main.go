package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"git.meow.tf/ow-api/ow-api/cache"
	"git.meow.tf/ow-api/ow-api/json-patch"
	"github.com/PuerkitoBio/goquery"
	"github.com/julienschmidt/httprouter"
	"github.com/ow-api/ovrstat/ovrstat"
	"github.com/rs/cors"
	"github.com/stoewer/go-strcase"
	"golang.org/x/net/context"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	Version = "2.4.7"

	OpAdd    = "add"
	OpRemove = "remove"
)

type ApiVersion int

const (
	VersionOne ApiVersion = iota
	VersionTwo
	VersionThree
)

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
	flagCacheTime = flag.Int("cacheTime", 300, "Cache time in seconds")

	cacheProvider cache.Provider

	cacheTime time.Duration

	profilePatch *jsonpatch.Patch

	heroNames []string

	platforms = []string{ovrstat.PlatformPC, ovrstat.PlatformConsole}
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

	router.HEAD("/status", statusHandler)
	router.GET("/status", statusHandler)

	registerVersionOne(router)

	registerVersionTwo(router)

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

func registerVersionOne(router *httprouter.Router) {
	for _, platform := range platforms {
		router.GET("/v1/stats/"+platform+"/:region/:tag/heroes/:heroes", injectPlatform(platform, heroes))
		router.GET("/v1/stats/"+platform+"/:region/:tag/profile", injectPlatform(platform, profile))
		router.GET("/v1/stats/"+platform+"/:region/:tag/complete", injectPlatform(platform, stats))
	}

	// Version
	router.GET("/v1/version", versionHandler)

	router.HEAD("/v1/status", statusHandler)
	router.GET("/v1/status", statusHandler)
}

func registerVersionTwo(router *httprouter.Router) {
	for _, platform := range platforms {
		router.GET("/v2/stats/"+platform+"/:tag/heroes/:heroes", injectPlatform(platform, heroes))
		router.GET("/v2/stats/"+platform+"/:tag/profile", injectPlatform(platform, profile))
		router.GET("/v2/stats/"+platform+"/:tag/complete", injectPlatform(platform, stats))
		router.GET("/v3/stats/"+platform+"/:tag/heroes/:heroes", injectPlatform(platform, heroes))
		router.GET("/v3/stats/"+platform+"/:tag/profile", injectPlatform(platform, profile))
		router.GET("/v3/stats/"+platform+"/:tag/complete", injectPlatform(platform, stats))
	}

	// Version
	router.GET("/v2/version", versionHandler)
	router.GET("/v3/version", versionHandler)
}

func loadHeroNames() {
	res, err := http.Get("https://overwatch.blizzard.com/en-us/heroes/")

	if err != nil {
		return
	}

	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)

	if err != nil {
		return
	}

	links := doc.Find(".heroCard")

	heroNames = make([]string, 0)

	links.Each(func(_ int, s *goquery.Selection) {
		val, exists := s.Attr("data-hero-id")

		if !exists {
			return
		}

		heroNames = append(heroNames, strcase.LowerCamelCase(val))
	})

	log.Println("Loaded heroes", heroNames)
}

var (
	versionRegexp = regexp.MustCompile("^/(v\\d+)/")
)

func injectPlatform(platform string, handler httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		if platform == "psn" || platform == "xbl" || platform == "nintendo-switch" {
			platform = ovrstat.PlatformConsole
		}

		ps = append(ps, httprouter.Param{Key: "platform", Value: platform})

		ctx := context.Background()

		m := versionRegexp.FindStringSubmatch(r.RequestURI)

		if m != nil {
			version := VersionOne

			switch m[1] {
			case "v2":
				version = VersionTwo
			case "v3":
				version = VersionThree
			}

			ctx = context.WithValue(ctx, "version", version)
		}

		handler(w, r.WithContext(ctx), ps)
	}
}

func statsResponse(w http.ResponseWriter, r *http.Request, ps httprouter.Params, patch *jsonpatch.Patch) ([]byte, error) {
	var stats *ovrstat.PlayerStats
	var err error

	version := VersionOne

	if v := r.Context().Value("version"); v != nil {
		version = v.(ApiVersion)
	}

	tag := ps.ByName("tag")

	tag = strings.Replace(tag, "#", "-", -1)

	cacheKey := generateCacheKey(r, ps)

	platform := ps.ByName("platform")

	stats, err = ovrstat.Stats(platform, strings.Replace(tag, "-", "#", -1))

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

		extra = append(extra, patchOperation{
			Op:    OpAdd,
			Path:  "/quickPlayStats/games",
			Value: games,
		})
	}

	if hs, ok := stats.CompetitiveStats.CareerStats["allHeroes"]; ok {
		games := &gamesStats{}

		games.Played = valueOrDefault(hs.Game, "gamesPlayed", 0)
		games.Won = valueOrDefault(hs.Game, "gamesWon", 0)

		extra = append(extra, patchOperation{
			Op:    OpAdd,
			Path:  "/competitiveStats/games",
			Value: games,
		})
	}

	if len(stats.Ratings) > 0 {
		if version == VersionThree {
			m := make(map[string]ovrstat.Rating)

			ratingsPatches := make([]patchOperation, len(stats.Ratings))

			for i, rating := range stats.Ratings {
				m[rating.Role] = rating
				ratingsPatches[i] = patchOperation{
					Op:   OpRemove,
					Path: "/ratings/" + rating.Role + "/role",
				}
			}

			extra = append(extra, patchOperation{
				Op:   OpRemove,
				Path: "/ratings",
			}, patchOperation{
				Op:    OpAdd,
				Path:  "/ratings",
				Value: m,
			})

			extra = append(extra, ratingsPatches...)
		}
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

func generateCacheKey(r *http.Request, ps httprouter.Params) string {
	version := VersionOne

	if v := r.Context().Value("version"); v != nil {
		version = v.(ApiVersion)
	}

	return versionToString(version) + "-" + ps.ByName("platform") + "-" + ps.ByName("tag")
}

func versionToString(version ApiVersion) string {
	return fmt.Sprintf("v%d", version)
}
