package thirdparty

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
)

// JiraTickets marshal to and unmarshal from the json issue
// returned by the rest api at /rest/api/latest/issue/{ticket_id}
type JiraTicket struct {
	jiraBase
	Key    string        `json:"key"`
	Expand string        `json:"expand"`
	Fields *TicketFields `json:"fields"`
}

// JiraSearchResults marshal to and unmarshal from the json
// search results returned by the rest api at /rest/api/2/search?jql={jql}
type JiraSearchResults struct {
	Expand     string       `json:"expand"`
	StartAt    int          `json:"startAt"`
	MaxResults int          `json:"maxResults"`
	Total      int          `json:"total"`
	Issues     []JiraTicket `json:"issues"`
}

type TicketFields struct {
	IssueType   *TicketType  `json:"issuetype"`
	Summary     string       `json:"summary"`
	Description string       `json:"description"`
	Reporter    *User        `json:"reporter"`
	Assignee    *User        `json:"assignee"`
	Project     *JiraProject `json:"project"`
	Created     string       `json:"created"`
	Updated     string       `json:"updated"`
	Status      *JiraStatus  `json:"status"`
}

type jiraBase struct {
	Id   string `json:"id"`
	Self string `json:"self"`
}

type JiraStatus struct {
	jiraBase
	Name string `json:"name"`
}

type TicketType struct {
	jiraBase
	Description string `json:"description"`
	IconUrl     string `json:"iconUrl"`
	Name        string `json:"name"`
	Subtask     bool   `json:"subtask"`
}

type JiraProject struct {
	jiraBase
	Key        string            `json:"key"`
	Name       string            `json:"name"`
	AvatarUrls map[string]string `json:"avatarUrls"`
}

type User struct {
	Self         string            `json:"self"`
	Name         string            `json:"name"`
	EmailAddress string            `json:"emailAddress"`
	DisplayName  string            `json:"displayName"`
	Active       bool              `json:"active"`
	TimeZone     string            `json:"timeZone"`
	AvatarUrls   map[string]string `json:"avatarUrls"`
}

type JiraHandler struct {
	MyHttp     httpGet
	JiraServer string
	UserName   string
	Password   string
}

func (jiraHandler *JiraHandler) GetJIRATicket(key string) (*JiraTicket, error) {
	apiEndpoint := fmt.Sprintf("https://%v/rest/api/latest/issue/%v", jiraHandler.JiraServer, url.QueryEscape(key))

	res, err := jiraHandler.MyHttp.doGet(apiEndpoint, jiraHandler.UserName, jiraHandler.Password)
	if res != nil {
		defer res.Body.Close()
	}

	if err != nil {
		return nil, err
	}

	if res == nil {
		return nil, fmt.Errorf("HTTP results are nil even though err was nil")
	}

	if res.StatusCode >= 300 || res.StatusCode < 200 {
		return nil, fmt.Errorf("HTTP request returned unexpected status `%v`", res.Status)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Unable to read http body: %v", err.Error())
	}

	ticket := &JiraTicket{}
	err = json.Unmarshal(body, ticket)
	if err != nil {
		return nil, err
	}

	return ticket, nil
}

// JQLSearch runs the given JQL query against the given jira instance and returns
// the results in a JiraSearchResults
func (jiraHandler *JiraHandler) JQLSearch(query string) (*JiraSearchResults, error) {
	apiEndpoint := fmt.Sprintf("https://%v/rest/api/latest/search?jql=%v", jiraHandler.JiraServer, url.QueryEscape(query))

	res, err := jiraHandler.MyHttp.doGet(apiEndpoint, jiraHandler.UserName, jiraHandler.Password)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("HTTP results are nil even though err was not nil")
	}

	if res.StatusCode >= 300 || res.StatusCode < 200 {
		return nil, fmt.Errorf("HTTP request returned unexpected status `%v`", res.Status)
	}

	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Unable to read http body: %v", err.Error())
	}

	results := &JiraSearchResults{}
	err = json.Unmarshal(body, results)
	if err != nil {
		return nil, err
	}

	return results, nil
}

func NewJiraHandler(server string, user string, password string) JiraHandler {
	return JiraHandler{
		liveHttpGet{},
		server,
		user,
		password,
	}
}
