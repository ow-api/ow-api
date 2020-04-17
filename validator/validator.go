package validator

import (
	"encoding/json"
	"errors"
	"github.com/PuerkitoBio/goquery"
	"log"
	"net/http"
	"regexp"
	"strings"
)

const (
	baseURL = "https://playoverwatch.com/en-us/career"

	apiURL = "https://playoverwatch.com/en-us/search/account-by-name/"
)

type Platform struct {
	Platform    string `json:"platform"`
	ID          int    `json:"id"`
	Name        string `json:"name"`
	URLName     string `json:"urlName"`
	PlayerLevel int    `json:"playerLevel"`
	Portrait    string `json:"portrait"`
	IsPublic    bool   `json:"isPublic"`
}

var (
	careerInitRegexp = regexp.MustCompile(`window\.app\.career\.init\((\d+)\,`)

	// Response/server errors
	errInvalidResponse     = errors.New("error querying the response")
	errUnexpectedStatus    = errors.New("unexpected status code")
	errRequestBlocked      = errors.New("request blocked")
	errNoApiResponse       = errors.New("unable to contact api endpoint")
	errApiInvalidJson      = errors.New("invalid json response")
	errApiUnexpectedStatus = errors.New("unexpected status code from api")

	// Invalid/missing elements
	errNoMasthead   = errors.New("unable to find masthead")
	errNoCareerInit = errors.New("no career init call found")

	// Element definitions
	mastheadElements = []string{
		"img.player-portrait",
		"div.player-level div.u-vertical-center",
		"div.player-level",
		"div.player-rank",
		"div.EndorsementIcon-tooltip div.u-center",
		"div.EndorsementIcon",
		"div.masthead p.masthead-detail.h4 span",
	}
)

func ValidateEndpoint() error {
	url := baseURL + "/pc/cats-11481"

	res, err := http.Get(url)

	if err != nil {
		return errInvalidResponse
	}

	if res.StatusCode == http.StatusForbidden {
		return errRequestBlocked
	} else if res.StatusCode != http.StatusOK {
		return errUnexpectedStatus
	}

	defer res.Body.Close()

	pd, err := goquery.NewDocumentFromReader(res.Body)

	if err != nil {
		return errInvalidResponse
	}

	masthead := pd.Find("div.masthead")

	if masthead.Length() < 1 {
		return errNoMasthead
	}

	// Validate masthead elements
	if err := validateElementsExist(masthead.First(), "div.masthead", mastheadElements...); err != nil {
		return err
	}

	if err != nil {
		return err
	}

	// Validate API response
	if err := validateApi(strings.Replace(url[strings.LastIndex(url, "/")+1:], "-", "%23", -1)); err != nil {
		return err
	}

	// Validate specific elements
	if err := validateElementsExist(pd.Selection, "", "div#quickplay", "div#competitive"); err != nil {
		return err
	}

	if err := validateDetailedStats(pd.Find("div#quickplay").First(), "div#quickplay"); err != nil {
		return err
	}

	if err := validateDetailedStats(pd.Find("div#competitive").First(), "div#competitive"); err != nil {
		return err
	}

	return nil
}

func validateDetailedStats(s *goquery.Selection, parent string) error {
	if err := validateElementsExist(s, parent, "div.progress-category", "div.js-stats"); err != nil {
		return err
	}

	var err error

	// Validate hero stats
	err = validateHeroStats(s.Find("div.progress-category").Parent(), parent)

	if err != nil {
		log.Println("No hero stats: " + err.Error())
		return err
	}

	return validateCareerStats(s.Find("div.js-stats").Parent(), parent)
}

func validateHeroStats(s *goquery.Selection, parent string) error {
	selectors := []string{
		"div.progress-category",                                             // Top level
		"div.progress-category div.ProgressBar",                             // ProgressBar
		"div.progress-category div.ProgressBar div.ProgressBar-title",       // ProgressBar Title
		"div.progress-category div.ProgressBar div.ProgressBar-description", // ProgressBar Description
	}

	return validateElementsExist(s, parent, selectors...)
}

func validateCareerStats(careerStatsSelector *goquery.Selection, parent string) error {
	if err := validateElementsExist(careerStatsSelector, parent, "select option"); err != nil {
		return err
	}

	selectors := []string{
		"div.row div.js-stats",                                        // Top level
		"div.row div.js-stats div.column",                             // stat boxes
		"div.row div.js-stats div.column .stat-title",                 // stat boxes
		"div.row div.js-stats div.column table.DataTable",             // data table
		"div.row div.js-stats div.column table.DataTable tbody",       // data table tbody
		"div.row div.js-stats div.column table.DataTable tbody tr",    // data table tbody tr
		"div.row div.js-stats div.column table.DataTable tbody tr td", // data table tbody tr td
	}

	return validateElementsExist(careerStatsSelector, parent, selectors...)
}

func validateApi(code string) error {
	var platforms []Platform

	apires, err := http.Get(apiURL + code)

	if err != nil {
		return errNoApiResponse
	}

	defer apires.Body.Close()

	if apires.StatusCode != http.StatusOK {
		return errApiUnexpectedStatus
	}

	if err := json.NewDecoder(apires.Body).Decode(&platforms); err != nil {
		return errApiInvalidJson
	}

	return nil
}

func validateElementsExist(s *goquery.Selection, parent string, elements ...string) error {
	var e *goquery.Selection

	for _, selector := range elements {
		e = s.Find(selector)

		if e.Length() < 1 {
			return errors.New("unable to find element: " + parent + " " + selector)
		}
	}

	return nil
}
