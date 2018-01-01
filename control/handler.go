package control

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"time"

	"github.com/go-chi/chi"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"

	"github.com/ichiban/jesi/cache"
	"github.com/ichiban/jesi/transaction"
)

const (
	authorization = "Authorization"

	methodPurge = "PURGE"
)

var (
	authorizationPattern = regexp.MustCompile(`\A\s*(?i:bearer)\s+([[:alnum:]-._~+/]+)\s*\z`)
)

// Handler handles control requests with bearer token.
type Handler struct {
	*cache.Store
	*chi.Mux

	Secret string
	Next   http.Handler
}

var _ http.Handler = (*Handler)(nil)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.init()

	h.Mux.ServeHTTP(w, r)
}

func init() {
	chi.RegisterMethod(methodPurge)
}

func (h *Handler) init() {
	if h.Mux != nil {
		return
	}
	h.Mux = chi.NewMux()
	h.Mux.Use(h.authorize)
	h.Mux.Route("/_jesi", func(r chi.Router) {
		r.Get("/resources", h.handleResources)
		r.Get("/resources/{path:.*}", h.handleResource)
		r.Get("/reps/{id:.*}", h.handleRepresentation)
		r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
			// TODO: prometheus metrics
			w.WriteHeader(http.StatusNotFound)
		})
		r.NotFound(http.NotFound)
	})
	h.Mux.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == methodPurge {
			h.handlePurge(w, r)
			return
		}
		h.Next.ServeHTTP(w, r)
	})
}

func (h *Handler) authorize(base http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.Secret == "" {
			log.WithFields(log.Fields{
				"id": transaction.ID(r),
			}).Debug("secret is not given")

			h.Next.ServeHTTP(w, r)
			return
		}

		m := authorizationPattern.FindStringSubmatch(r.Header.Get(authorization))
		if len(m) != 2 {
			log.WithFields(log.Fields{
				"id": transaction.ID(r),
			}).Debug("Authorization header doesn't match")

			h.Next.ServeHTTP(w, r)
			return
		}
		if h.Secret != m[1] {
			log.WithFields(log.Fields{
				"id": transaction.ID(r),
			}).Debug("secret doesn't match")

			h.Next.ServeHTTP(w, r)
			return
		}
		base.ServeHTTP(w, r)
	})
}

func (h *Handler) handlePurge(w http.ResponseWriter, r *http.Request) {
	res := h.Purge(r)
	if res == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	reps := make([]representation, 0, len(res.Representations))
	for _, rep := range res.Representations {
		reps = append(reps, representation{
			Links: representationLinks{
				Self:     link{Href: representationURL(rep).String()},
				Resource: link{Href: resourceURL(res).String()},
			},
			Status:        rep.StatusCode,
			Header:        rep.HeaderMap,
			ContentLength: len(rep.Body),
			RequestTime:   rep.RequestTime.Format(time.RFC3339),
			ResponseTime:  rep.ResponseTime.Format(time.RFC3339),
			LastUsedTime:  rep.LastUsedTime.Format(time.RFC3339),
		})
	}

	b, err := json.Marshal(resource{
		Links: resourceLinks{
			Self:  link{Href: resourceURL(res).String()},
			About: link{Href: aboutURL(res).String()},
		},
		Embed: &resourceEmbed{
			Representations: reps,
		},
		Unique: res.Unique,
		Fields: res.Fields,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

func (h *Handler) handleResources(w http.ResponseWriter, r *http.Request) {
	h.RLock()
	defer h.RUnlock()

	links := make([]link, 0, len(h.Resources))
	for _, res := range h.Resources {
		links = append(links, link{Href: resourceURL(res).String()})
	}

	c := resourceCollection{
		Links: resourceCollectionLinks{
			Self:     link{Href: r.URL.String()},
			Elements: links,
		},
	}

	b, err := json.Marshal(c)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("failed to marshal resource collection")
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/hal+json")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

type resourceCollection struct {
	Links resourceCollectionLinks `json:"_links"`
}

type resourceCollectionLinks struct {
	Self     link   `json:"self"`
	Elements []link `json:"elements"`
}

func (h *Handler) handleResource(w http.ResponseWriter, r *http.Request) {
	key := cache.ResourceKey{
		Host:  r.URL.Host,
		Path:  "/" + chi.URLParam(r, "path"),
		Query: r.URL.Query().Encode(),
	}

	h.RLock()
	defer h.RUnlock()

	res, ok := h.Resources[key]
	if !ok {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	links := make([]link, 0, len(res.Representations))
	for _, r := range res.Representations {
		links = append(links, link{
			Href: representationURL(r).String(),
		})
	}

	m := resource{
		Links: resourceLinks{
			Self:            link{Href: resourceURL(res).String()},
			About:           link{Href: aboutURL(res).String()},
			Representations: links,
		},
		Unique: res.Unique,
		Fields: res.Fields,
	}

	b, err := json.Marshal(m)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("failed to marshal resource")
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/hal+json")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

func (h *Handler) handleRepresentation(w http.ResponseWriter, r *http.Request) {
	h.Store.Init()

	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	rep, ok := h.Representations[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	res := h.Resources[rep.ResourceKey]
	m := representation{
		Links: representationLinks{
			Self:     link{Href: representationURL(rep).String()},
			Resource: link{Href: resourceURL(res).String()},
		},
		Status:        rep.StatusCode,
		Header:        rep.HeaderMap,
		ContentLength: len(rep.Body),
		RequestTime:   rep.RequestTime.Format(time.RFC3339),
		ResponseTime:  rep.ResponseTime.Format(time.RFC3339),
		LastUsedTime:  rep.LastUsedTime.Format(time.RFC3339),
	}

	b, err := json.Marshal(m)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("failed to marshal representation")
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/hal+json")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

type resource struct {
	Links  resourceLinks  `json:"_links"`
	Embed  *resourceEmbed `json:"_embed,omitempty"`
	Unique bool           `json:"unique"`
	Fields []string       `json:"fields"`
}

type resourceLinks struct {
	Self            link   `json:"self"`
	About           link   `json:"about"`
	Representations []link `json:"reps,omitempty"`
}

type resourceEmbed struct {
	Representations []representation `json:"reps"`
}

type representation struct {
	Links         representationLinks `json:"_links"`
	Status        int                 `json:"status"`
	Header        http.Header         `json:"header"`
	ContentLength int                 `json:"contentLength"`
	RequestTime   string              `json:"requestTime"`
	ResponseTime  string              `json:"responseTime"`
	LastUsedTime  string              `json:"lastUsedTime"`
}

type representationLinks struct {
	Self     link `json:"self"`
	Resource link `json:"resource"`
}

type link struct {
	Href string `json:"href"`
}

func resourceURL(res *cache.Resource) *url.URL {
	return &url.URL{
		Host:     res.Host,
		Path:     path.Join("/_jesi/resources", res.Path),
		RawQuery: res.Query,
	}
}

func representationURL(rep *cache.Representation) *url.URL {
	return &url.URL{
		Host: rep.Host,
		Path: path.Join("/_jesi/reps/%s", rep.ID.String()),
	}
}

func aboutURL(res *cache.Resource) *url.URL {
	return &url.URL{
		Host:     res.Host,
		Path:     res.Path,
		RawQuery: res.Query,
	}
}
