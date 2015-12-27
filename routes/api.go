package routes

import (
	"database/sql"
	"encoding/json"
	"github.com/MarvinMenzerath/UpAndRunning2/lib"
	"github.com/julienschmidt/httprouter"
	"github.com/op/go-logging"
	"net/http"
	"strconv"
)

// Sends a simple welcome-message to the user.
func ApiIndex(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	SendJsonMessage(w, http.StatusOK, true, "Welcome to UpAndRunning2's API!")
}

// Returns a DetailedWebsiteResponse containing all the Website's important data if the Website is enabled.
func ApiStatus(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var (
		id                   int
		name                 string
		protocol             string
		url                  string
		statusCode           string
		statusText           string
		responseTime         string
		time                 string
		lastFailStatusCode   string
		lastFailStatusText   string
		lastFailResponseTime string
		lastFailTime         string
		ups                  int
		totalChecks          int
	)

	// Query the Database for basic data and the last successful check
	db := lib.GetDatabase()
	err := db.QueryRow("SELECT websites.id, websites.name, websites.protocol, websites.url, checks.statusCode, checks.statusText, checks.responseTime, checks.time FROM checks, websites WHERE checks.websiteId = websites.id AND websites.url = ? AND websites.enabled = 1 ORDER BY checks.id DESC LIMIT 1;", ps.ByName("url")).Scan(&id, &name, &protocol, &url, &statusCode, &statusText, &responseTime, &time)
	switch {
	case err == sql.ErrNoRows:
		SendJsonMessage(w, http.StatusNotFound, false, "Unable to find any data matching the given url.")
		return
	case err != nil:
		logging.MustGetLogger("logger").Error("Unable to fetch Website-Status: ", err)
		SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
		return
	}

	// Query the Database for the last unsuccessful check
	err = db.QueryRow("SELECT checks.statusCode, checks.statusText, checks.responseTime, checks.time FROM checks, websites WHERE checks.websiteId = websites.id AND (checks.statusCode NOT LIKE '2%' AND checks.statusCode NOT LIKE '3%') AND websites.url = ? AND websites.enabled = 1 ORDER BY checks.id DESC LIMIT 1;", ps.ByName("url")).Scan(&lastFailStatusCode, &lastFailStatusText, &lastFailResponseTime, &lastFailTime)
	switch {
	case err == sql.ErrNoRows:
		lastFailStatusCode = "0"
		lastFailStatusText = "unknown"
		lastFailResponseTime = "0"
		lastFailTime = "0000-00-00 00:00:00"
	case err != nil:
		logging.MustGetLogger("logger").Error("Unable to fetch Website-Status: ", err)
		SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
		return
	}

	// Query the Database for the amount of (successful / total) checks
	err = db.QueryRow("SELECT (SELECT COUNT(checks.id) FROM checks, websites WHERE checks.websiteId = websites.id AND (checks.statusCode LIKE '2%' OR checks.statusCode LIKE '3%') AND websites.url = ?) AS ups, (SELECT COUNT(checks.id) FROM checks, websites WHERE checks.websiteId = websites.id AND websites.url = ?) AS total FROM checks LIMIT 1;", ps.ByName("url"), ps.ByName("url")).Scan(&ups, &totalChecks)
	switch {
	case err == sql.ErrNoRows:
		logging.MustGetLogger("logger").Error("Unable to fetch Website-Status: ", err)
		SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
		return
	case err != nil:
		logging.MustGetLogger("logger").Error("Unable to fetch Website-Status: ", err)
		SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
		return
	}

	// Build Response
	responseJson := DetailedWebsiteResponse{true, WebsiteData{id, name, protocol + "://" + url}, WebsiteAvailability{ups, totalChecks - ups, totalChecks, strconv.FormatFloat((float64(ups)/float64(totalChecks))*100, 'f', 2, 64) + "%"}, WebsiteCheckResult{statusCode + " - " + statusText, responseTime + " ms", time}, WebsiteCheckResult{lastFailStatusCode + " - " + lastFailStatusText, lastFailResponseTime + " ms", lastFailTime}}

	// Send Response
	responseBytes, err := json.Marshal(responseJson)
	if err != nil {
		SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(responseBytes)
}

// Returns a ResultsResponse containing an array of WebsiteCheckResults.
func ApiResults(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// Get limit-parameter from Request
	limit := 100
	limitString := r.URL.Query().Get("limit")
	if len(limitString) != 0 {
		parsedLimit, err := strconv.Atoi(limitString)
		if err != nil {
			SendJsonMessage(w, http.StatusBadRequest, false, "Unable to parse given limit-parameter.")
			return
		}
		if parsedLimit > 9999 {
			SendJsonMessage(w, http.StatusBadRequest, false, "Unable to process your Request: Limit has to be less than 10000.")
			return
		}
		limit = parsedLimit
	}

	// Get offset-parameter from Request
	offset := 0
	offsetString := r.URL.Query().Get("offset")
	if len(offsetString) != 0 {
		parsedOffset, err := strconv.Atoi(offsetString)
		if err != nil {
			SendJsonMessage(w, http.StatusBadRequest, false, "Unable to parse given offset-parameter.")
			return
		}
		if parsedOffset > 9999 {
			SendJsonMessage(w, http.StatusBadRequest, false, "Unable to process your Request: Offset has to be less than 10000.")
			return
		}
		offset = parsedOffset
	}

	// Query the Database
	db := lib.GetDatabase()
	rows, err := db.Query("SELECT statusCode, statusText, responseTime, time FROM checks, websites WHERE checks.websiteId = websites.id AND websites.url = ? AND websites.enabled = 1 ORDER BY time DESC LIMIT ? OFFSET ?;", ps.ByName("url"), limit, offset)
	if err != nil {
		logging.MustGetLogger("logger").Error("Unable to fetch Results: ", err)
		SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
		return
	}
	defer rows.Close()

	// Add every Result
	results := []WebsiteCheckResult{}
	var (
		statusCode string
		statusText string
		responseTime string
		time string
	)
	for rows.Next() {
		err = rows.Scan(&statusCode, &statusText, &responseTime, &time)
		if err != nil {
			logging.MustGetLogger("logger").Error("Unable to read Result-Row: ", err)
			SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
			return
		}

		results = append(results, WebsiteCheckResult{statusCode + " - " + statusText, responseTime, time})
	}

	// Check for Errors
	err = rows.Err()
	if err != nil {
		logging.MustGetLogger("logger").Error("Unable to read Result-Rows: ", err)
		SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
		return
	}

	// Send Response
	responseBytes, err := json.Marshal(ResultsResponse{true, results})
	if err != nil {
		SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(responseBytes)
}

// Returns a WebsiteResponse containing all publicly visible Websites as BasicWebsite.
func ApiWebsites(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// Query the Database for basic data
	db := lib.GetDatabase()
	rows, err := db.Query("SELECT name, protocol, url FROM websites WHERE enabled = 1 AND visible = 1 ORDER BY name;")
	if err != nil {
		logging.MustGetLogger("logger").Error("Unable to fetch Websites: ", err)
		SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
		return
	}
	defer rows.Close()

	// Add every Website
	websites := []BasicWebsite{}
	var (
		name       string
		protocol   string
		url        string
		statusCode string
		statusText string
	)

	totalRows := 0
	for rows.Next() {
		err = rows.Scan(&name, &protocol, &url)
		if err != nil {
			logging.MustGetLogger("logger").Error("Unable to read Website-Data-Row: ", err)
			SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
			return
		}

		websites = append(websites, BasicWebsite{name, protocol, url, ""})
		totalRows++
	}

	// Query the database for status data
	rows, err = db.Query("SELECT statusCode, statusText FROM (SELECT name, statusCode, statusText FROM checks, websites WHERE checks.websiteId = websites.id AND enabled = 1 AND visible = 1 ORDER BY checks.id DESC LIMIT ?) AS t ORDER BY name;", totalRows)
	if err != nil {
		logging.MustGetLogger("logger").Error("Unable to fetch Websites: ", err)
		SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
		return
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		err = rows.Scan(&statusCode, &statusText)
		if err != nil {
			logging.MustGetLogger("logger").Error("Unable to read Website-Status-Row: ", err)
			SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
			return
		}

		websites[i].Status = statusCode + " - " + statusText
		i++
	}

	// Send Response
	responseBytes, err := json.Marshal(WebsiteResponse{true, websites})
	if err != nil {
		SendJsonMessage(w, http.StatusInternalServerError, false, "Unable to process your Request.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(responseBytes)
}
