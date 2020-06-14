package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"git.meow.tf/ow-api/ow-api/cache"
	"git.meow.tf/ow-api/ow-api/json-patch"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/cors"
	"golang.org/x/net/context"
	"log"
	"net/http"
	"regexp"
	"s32x.com/ovrstat/ovrstat"
	"strings"
	"time"
)

const (
	Version = "2.3.6"

	OpAdd    = "add"
	OpRemove = "remove"
)

type ApiVersion int

const (
	VersionOne ApiVersion = iota
	VersionTwo
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

	platforms = []string{ovrstat.PlatformPC, ovrstat.PlatformXBL, ovrstat.PlatformPSN, ovrstat.PlatformNS}
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
	}

	// Version
	router.GET("/v2/version", versionHandler)
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

var (
	versionRegexp = regexp.MustCompile("^/(v\\d+)/")
)

func injectPlatform(platform string, handler httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ps = append(ps, httprouter.Param{Key: "platform", Value: platform})

		ctx := context.Background()

		m := versionRegexp.FindStringSubmatch(r.RequestURI)

		if m != nil {
			version := VersionOne

			switch m[1] {
			case "v2":
				version = VersionTwo
			}

			ctx = context.WithValue(ctx, "version", version)
		}

		handler(w, r.WithContext(ctx), ps)
	}
}

var (
	tagRegexp = regexp.MustCompile("-(\\d+)$")
)

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

	switch platform {
	case ovrstat.PlatformPC:
		if !tagRegexp.MatchString(tag) {
			w.WriteHeader(http.StatusBadRequest)
			return nil, errors.New("bad tag")
		}
		stats, err = ovrstat.PCStats(tag)
	case ovrstat.PlatformPSN, ovrstat.PlatformXBL, ovrstat.PlatformNS:
		stats, err = ovrstat.ConsoleStats(platform, tag)
	default:
		return nil, errors.New("unknown platform")
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

	rating := 0
	var ratingIcon string

	if len(stats.Ratings) > 0 {
		totalRating := 0
		iconUrl := ""

		for _, rating := range stats.Ratings {
			totalRating += rating.Level
			iconUrl = rating.RankIcon
		}

		rating = int(totalRating / len(stats.Ratings))

		urlBase := iconUrl[0 : strings.Index(iconUrl, "rank-icons/")+11]

		ratingIcon = urlBase + iconFor(rating)

		if version == VersionTwo {
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

	extra = append(extra, patchOperation{
		Op:    OpAdd,
		Path:  "/rating",
		Value: rating,
	}, patchOperation{
		Op:    OpAdd,
		Path:  "/ratingIcon",
		Value: ratingIcon,
	})

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

func iconFor(rating int) string {
	if rating >= 4000 {
		return "rank-GrandmasterTier.png"
	} else if rating >= 3500 {
		return "rank-MasterTier.png"
	} else if rating >= 3000 {
		return "rank-DiamondTier.png"
	} else if rating >= 2500 {
		return "rank-PlatinumTier.png"
	} else if rating >= 2000 {
		return "rank-GoldTier.png"
	} else if rating >= 1500 {
		return "rank-SilverTier.png"
	}

	return "rank-BronzeTier.png"
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
