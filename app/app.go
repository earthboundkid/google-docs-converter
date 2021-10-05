package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"

	"github.com/carlmjohnson/flagext"
	"github.com/carlmjohnson/requests"
	"golang.org/x/net/html"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
)

const AppName = "gdocs"

func CLI(args []string) error {
	var app appEnv
	err := app.ParseArgs(args)
	if err != nil {
		return err
	}
	err = app.Exec()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	return err
}

func (app *appEnv) ParseArgs(args []string) error {
	fl := flag.NewFlagSet(AppName, flag.ContinueOnError)
	fl.StringVar(&app.docid, "id", "", "ID for Google Doc")
	fl.StringVar(&app.outputDoc, "write-doc", "", "`path` to write out document")
	fl.StringVar(&app.inputDoc, "read-doc", "", "`path` to read document from instead of Google Docs")
	fl.StringVar(&app.oauthClientID, "oauth-client-id", "", "client `id` for Google OAuth 2.0 authentication")
	fl.StringVar(&app.oauthClientSecret, "oauth-client-secret", "", "client `secret` for Google OAuth 2.0 authentication")

	app.Logger = log.New(nil, AppName+" ", log.LstdFlags)
	fl.Var(
		flagext.Logger(app.Logger, flagext.LogSilent),
		"silent",
		`don't log debug output`,
	)

	fl.Usage = func() {
		fmt.Fprintf(fl.Output(), `gdocs %s - extracts a document from Google Docs

Usage:

	gdocs [options]

Uses Google default credentials if no Oauth credentials are provided. See

https://developers.google.com/accounts/docs/application-default-credentials
https://developers.google.com/identity/protocols/oauth2

Options:
`, getVersion())
		fl.PrintDefaults()
		fmt.Fprintln(fl.Output(), "")
	}
	if err := fl.Parse(args); err != nil {
		return err
	}
	if err := flagext.ParseEnv(fl, AppName); err != nil {
		return err
	}

	app.docid = normalizeID(app.docid)
	return nil
}

func getVersion() string {
	if i, ok := debug.ReadBuildInfo(); ok {
		return i.Main.Version
	}

	return "(unknown)"
}

type appEnv struct {
	docid             string
	oauthClientID     string
	oauthClientSecret string
	inputDoc          string
	outputDoc         string
	*log.Logger
}

func (app *appEnv) Exec() (err error) {
	app.Println("starting Google Docs service")
	ctx := context.Background()
	var doc docs.Document
	if app.inputDoc == "" {
		getClient := app.oauthClient
		if app.oauthClientID == "" || app.oauthClientSecret == "" {
			getClient = app.defaultCredentials
		}
		client, err := getClient(ctx)
		if err != nil {
			return err
		}
		app.Printf("getting %q", app.docid)
		err = requests.
			URL("https://docs.googleapis.com").
			Pathf("/v1/documents/%s", app.docid).
			Client(client).
			ToJSON(&doc).
			Fetch(ctx)
		if err != nil {
			return err
		}
		if app.outputDoc != "" {
			b, err := json.MarshalIndent(&doc, "", "  ")
			if err != nil {
				return err
			}

			if err = os.WriteFile(app.outputDoc, b, 0644); err != nil {
				return err
			}
		}
	} else {
		app.Printf("reading %q", app.inputDoc)
		b, err := os.ReadFile(app.inputDoc)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, &doc); err != nil {
			return err
		}
	}

	app.Printf("got %q", doc.Title)

	n := convert(&doc)

	return html.Render(os.Stdout, n)
}

func normalizeID(id string) string {
	id = strings.TrimPrefix(id, "https://docs.google.com/document/d/")
	id, _, _ = cut(id, "/")
	return id
}

func convert(doc *docs.Document) (n *html.Node) {
	n = &html.Node{
		Type: html.DocumentNode,
	}
	listInfo := buildListInfo(doc.Lists)
	objectInfo := buildObjectInfo(doc.InlineObjects)
	for _, el := range doc.Body.Content {
		convertEl(n, el, listInfo, objectInfo)
	}
	return
}

var tagForNamedStyle = map[string]string{
	"NAMED_STYLE_TYPE_UNSPECIFIED": "div",
	"NORMAL_TEXT":                  "p",
	"TITLE":                        "h1",
	"SUBTITLE":                     "h1",
	"HEADING_1":                    "h1",
	"HEADING_2":                    "h2",
	"HEADING_3":                    "h3",
	"HEADING_4":                    "h4",
	"HEADING_5":                    "h5",
	"HEADING_6":                    "h6",
}

func buildListInfo(lists map[string]docs.List) map[string]string {
	m := map[string]string{}
	for id, list := range lists {
		if list.ListProperties == nil {
			continue
		}
		listType := "ul"
		if list.ListProperties.NestingLevels[0].GlyphType != "" {
			listType = "ol"
		}
		m[id] = listType
	}
	return m
}

func buildObjectInfo(objs map[string]docs.InlineObject) map[string][]string {
	m := map[string][]string{}
	for id, obj := range objs {
		if obj.InlineObjectProperties == nil {
			continue
		}
		innerObj := obj.InlineObjectProperties.EmbeddedObject
		src := ""
		if innerObj.ImageProperties != nil {
			src = innerObj.ImageProperties.ContentUri
		}
		m[id] = []string{
			"src", src,
			"title", innerObj.Title,
			"alt", innerObj.Description,
		}
	}
	return m
}

func convertEl(n *html.Node, el *docs.StructuralElement, listInfo map[string]string, objInfo map[string][]string) {
	if el.Table != nil && el.Table.TableRows != nil {
		table := newElement("table")
		n.AppendChild(table)
		for _, row := range el.Table.TableRows {
			rowEl := newElement("tr")
			table.AppendChild(rowEl)
			if row.TableCells != nil {
				for _, cell := range row.TableCells {
					cellEl := newElement("td")
					rowEl.AppendChild(cellEl)
					for _, content := range cell.Content {
						convertEl(cellEl, content, listInfo, objInfo)
					}
				}
			}
		}
	}
	if el.Paragraph == nil {
		return
	}
	if el.Paragraph.Bullet != nil {
		listType := listInfo[el.Paragraph.Bullet.ListId]
		ul := lastChildOrNewElement(n, listType)
		li := newElement("li")
		ul.AppendChild(li)
		n = li
	}

	blockType := tagForNamedStyle[el.Paragraph.ParagraphStyle.NamedStyleType]

	n.AppendChild(newElement(blockType))

	for _, subel := range el.Paragraph.Elements {
		if subel.HorizontalRule != nil {
			n.AppendChild(newElement("hr"))
		}

		if subel.InlineObjectElement != nil {
			inner := lastChildOrNewElement(n, blockType)
			attrs := objInfo[subel.InlineObjectElement.InlineObjectId]
			inner.AppendChild(newElement("img", attrs...))
		}

		if subel.TextRun == nil {
			continue
		}

		inner := lastChildOrNewElement(n, blockType)
		if subel.TextRun.TextStyle != nil {
			if len(subel.TextRun.SuggestedInsertionIds) > 0 {
				newinner := newElement("ins")
				inner.AppendChild(newinner)
				inner = newinner
			}
			if len(subel.TextRun.SuggestedDeletionIds) > 0 {
				newinner := newElement("del")
				inner.AppendChild(newinner)
				inner = newinner
			}
			if subel.TextRun.TextStyle.Link != nil {
				newinner := newElement("a", "href", subel.TextRun.TextStyle.Link.Url)
				inner.AppendChild(newinner)
				inner = newinner
			}
			if subel.TextRun.TextStyle.BackgroundColor != nil {
				newinner := newElement("mark")
				inner.AppendChild(newinner)
				inner = newinner
			}
			if subel.TextRun.TextStyle.Bold {
				newinner := newElement("strong")
				inner.AppendChild(newinner)
				inner = newinner
			}
			if subel.TextRun.TextStyle.Italic {
				newinner := newElement("em")
				inner.AppendChild(newinner)
				inner = newinner
			}
			if subel.TextRun.TextStyle.Underline && subel.TextRun.TextStyle.Link == nil {
				newinner := newElement("u")
				inner.AppendChild(newinner)
				inner = newinner
			}
		}
		appendText(inner, subel.TextRun.Content)
	}
}

func newElement(tag string, attrs ...string) *html.Node {
	var attrslice []html.Attribute
	if len(attrs) > 0 {
		if len(attrs)%2 != 0 {
			panic("uneven number of attr/value pairs")
		}
		attrslice = make([]html.Attribute, len(attrs)/2)
		for i := range attrslice {
			attrslice[i] = html.Attribute{
				Key: attrs[i*2],
				Val: attrs[i*2+1],
			}
		}
	}
	return &html.Node{
		Type: html.ElementNode,
		Data: tag,
		Attr: attrslice,
	}
}

func lastChildOrNewElement(p *html.Node, tag string, attrs ...string) *html.Node {
	if p.LastChild != nil && p.LastChild.Data == tag {
		return p.LastChild
	}
	n := newElement(tag, attrs...)
	p.AppendChild(n)
	return n
}

func appendText(n *html.Node, text string) {
	n.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: text,
	})
}

var scopes = []string{
	"https://www.googleapis.com/auth/documents",
	"https://www.googleapis.com/auth/documents.readonly",
	"https://www.googleapis.com/auth/drive",
	"https://www.googleapis.com/auth/drive.file",
	"https://www.googleapis.com/auth/drive.readonly",
}

func (app *appEnv) defaultCredentials(ctx context.Context) (*http.Client, error) {
	app.Printf("using default credentials")
	return google.DefaultClient(ctx, scopes...)
}

func getToken() (string, error) {
	var b [15]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b[:]), nil
}

func (app *appEnv) oauthClient(ctx context.Context) (client *http.Client, err error) {
	app.Printf("using oauth credentials")
	stateToken, err := getToken()
	if err != nil {
		return nil, err
	}
	code := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("state") != stateToken {
			http.NotFound(w, r)
			return
		}
		code <- r.FormValue("code")
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<h1>Success</h1><p>You may close this window."))
	}))
	defer srv.Close()

	conf := &oauth2.Config{
		ClientID:     app.oauthClientID,
		ClientSecret: app.oauthClientSecret,
		RedirectURL:  srv.URL,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
	}
	// Redirect user to Google's consent page to ask for permission
	url := conf.AuthCodeURL(stateToken)
	if launcherr := exec.CommandContext(ctx, "open", url).Run(); launcherr != nil {
		fmt.Printf("Visit the URL for the auth dialog: %v", url)
	}
	tok, err := conf.Exchange(ctx, <-code)
	if err != nil {
		return nil, err
	}
	return conf.Client(ctx, tok), nil
}

// See https://github.com/golang/go/issues/46336
func cut(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}
