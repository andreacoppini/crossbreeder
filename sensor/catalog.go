package main

import (
	"sort"
	"strings"
)

// App is a hosted application a site depends on, and the endpoints that say
// whether it is reachable from here. Naming them in the configuration rather
// than pasting URLs is what makes a sensor's results comparable across a
// fleet: every sensor tests Microsoft 365 the same way.
type App struct {
	Name     string
	Category string
	Tests    []WebTarget
}

// The catalogue covers what sites actually complain about. Each entry points
// at an endpoint that is meant to be fetched — a status or discovery URL
// wherever one exists, rather than a marketing page that changes shape.
var appCatalogue = []App{
	{"Microsoft 365", "Productivity", []WebTarget{
		{Name: "Microsoft 365 sign-in", URL: "https://login.microsoftonline.com/common/discovery/keys", ExpectStatus: 200},
		{Name: "Exchange Online", URL: "https://outlook.office365.com/owa/", Follow: true},
	}},
	{"Microsoft Teams", "Collaboration", []WebTarget{
		{Name: "Teams", URL: "https://teams.microsoft.com/", Follow: true},
	}},
	{"Google", "Productivity", []WebTarget{
		{Name: "Google", URL: "https://www.google.com/generate_204", ExpectStatus: 204},
		{Name: "Gmail", URL: "https://mail.google.com/", Follow: true},
	}},
	{"Zoom", "Collaboration", []WebTarget{
		{Name: "Zoom", URL: "https://zoom.us/", Follow: true},
	}},
	{"Webex", "Collaboration", []WebTarget{
		{Name: "Webex", URL: "https://www.webex.com/", Follow: true},
	}},
	{"Slack", "Collaboration", []WebTarget{
		{Name: "Slack API", URL: "https://slack.com/api/api.test", ExpectStatus: 200, ExpectBody: `"ok":true`},
	}},
	{"Salesforce", "Business", []WebTarget{
		{Name: "Salesforce", URL: "https://login.salesforce.com/", Follow: true},
	}},
	{"ServiceNow", "Business", []WebTarget{
		{Name: "ServiceNow", URL: "https://www.servicenow.com/", Follow: true},
	}},
	{"Workday", "Business", []WebTarget{
		{Name: "Workday", URL: "https://www.workday.com/", Follow: true},
	}},
	{"Dropbox", "Storage", []WebTarget{
		{Name: "Dropbox", URL: "https://www.dropbox.com/", Follow: true},
	}},
	{"Box", "Storage", []WebTarget{
		{Name: "Box", URL: "https://account.box.com/login", Follow: true},
	}},
	{"AWS", "Cloud", []WebTarget{
		{Name: "AWS", URL: "https://aws.amazon.com/", Follow: true},
	}},
	{"Azure", "Cloud", []WebTarget{
		{Name: "Azure portal", URL: "https://portal.azure.com/", Follow: true},
	}},
	{"GitHub", "Development", []WebTarget{
		{Name: "GitHub API", URL: "https://api.github.com/", ExpectStatus: 200},
	}},
	{"Citrix", "Remote access", []WebTarget{
		{Name: "Citrix", URL: "https://www.citrix.com/", Follow: true},
	}},
	{"Cloudflare", "Internet", []WebTarget{
		{Name: "Cloudflare trace", URL: "https://www.cloudflare.com/cdn-cgi/trace", ExpectStatus: 200, ExpectBody: "fl="},
	}},
	{"Internet", "Internet", []WebTarget{
		{Name: "Connectivity check", URL: "http://connectivitycheck.gstatic.com/generate_204", ExpectStatus: 204},
	}},
}

// LookupApp finds an application by name, case and spacing insensitively, so
// "microsoft365" and "Microsoft 365" are the same entry.
func LookupApp(name string) (App, bool) {
	want := normaliseAppName(name)
	for _, a := range appCatalogue {
		if normaliseAppName(a.Name) == want {
			return a, true
		}
	}
	return App{}, false
}

func normaliseAppName(s string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.TrimSpace(s)))
}

// AppNames lists the catalogue for the command line and the dashboard.
func AppNames() []string {
	out := make([]string, 0, len(appCatalogue))
	for _, a := range appCatalogue {
		out = append(out, a.Name)
	}
	sort.Strings(out)
	return out
}

// AppsByCategory groups the catalogue for the configuration screen.
func AppsByCategory() map[string][]string {
	out := map[string][]string{}
	for _, a := range appCatalogue {
		out[a.Category] = append(out[a.Category], a.Name)
	}
	for _, names := range out {
		sort.Strings(names)
	}
	return out
}
