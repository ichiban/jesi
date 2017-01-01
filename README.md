# Jesi - a hypermedia API accelerator

Jesi (stands for JSON Edge Side Include) is an HTTP reverse proxy that accelerates your web API by embedding & caching JSON responses.

## Getting Started

TODO:
 
## Features

### Embedding

Jesi understands [JSON Hypertext Application Language aka HAL+JSON](http://tools.ietf.org/html/draft-kelly-json-hal) and can construct complex HAL+JSON documents out of simple HAL+JSON documents from the upstream server.
By supplying a query parameter `?with=<edges>` with dot separated edge names, it embeds HAL+JSON documents linked by `_links` as `_embeded`.

This will decrease the number of round trips over the Internet which is crucial for speeding up web API backed applications.

### Caching

Jesi implements HTTP Caching described in [RFC 7234](https://tools.ietf.org/html/rfc7234).
Every response including but not limited to HAL+JSON documents is cached and served from the cache on behalf of the upstream server while it's fresh.

Combined with embedding, the resulting HAL+JSON response is constructed from cached responses and responses newly fetched from the upstream server so that it can maximize cache effectiveness.

When Jesi cache reaches the memory limitation specified by `-max` command line option, it evicts some cached responses with LRU algorithm.

## Example

Let's consider an example of a movie database app. It has resources of a movie Pulp Fiction, roles Vincent Vega and Jules Winnfield, and actors John Travolta and Samuel L. Jackson.

```json
{
  "_links": {
    "roles": [
      {
        "href": "/roles/1"
      },
      {
        "href": "/roles/2"
      }
    ],
    "self": {
      "href": "/movies/1"
    }
  },
  "title": "Pulp Fiction",
  "year": 1994
}
```

```json
{
  "_links": {
    "actor": {
      "href": "/actors/1"
    },
    "movie": {
      "href": "/movies/1"
    },
    "self": {
      "href": "/roles/1"
    }
  },
  "name": "Vincent Vega"
}
```

```json
{
  "_links": {
    "actor": {
      "href": "/actors/2"
    },
    "movie": {
      "href": "/movies/1"
    },
    "self": {
      "href": "/roles/2"
    }
  },
  "name": "Jules Winnfield"
}
```

```json
{
  "_links": {
    "roles": [
      {
        "href": "/roles/1"
      }
    ],
    "self": {
      "href": "/actors/1"
    }
  },
  "name": "John Travolta"
}
```

```json
{
  "_links": {
    "roles": [
      {
        "href": "/roles/2"
      }
    ],
    "self": {
      "href": "/actors/2"
    }
  },
  "name": "Samuel L. Jackson"
}
```

They're connected by `_links` property - a movie has many roles and a role has exactly one actor.

![Movies](movies.png)

To render a view for Pulp Fiction, it has to make requests for the movie `/movies/1` and also `/roles/1`, `/roles/2`, `/actors/1`, and `/actors/2` for the details.
By making a request `/movies/1?with=roles.actor`, Jesi responds with one big HAL+JSON document with the roles and actors embedded in the movie JSON.

```json
{
  "_embedded": {
    "roles": [
      {
        "_embedded": {
          "actor": {
            "_links": {
              "roles": [
                {
                  "href": "/roles/1"
                }
              ],
              "self": {
                "href": "/actors/1"
              }
            },
            "name": "John Travolta"
          }
        },
        "_links": {
          "actor": {
            "href": "/actors/1"
          },
          "movie": {
            "href": "/movies/1"
          },
          "self": {
            "href": "/roles/1"
          }
        },
        "name": "Vincent Vega"
      },
      {
        "_embedded": {
          "actor": {
            "_links": {
              "roles": [
                {
                  "href": "/roles/2"
                }
              ],
              "self": {
                "href": "/actors/2"
              }
            },
            "name": "Samuel L. Jackson"
          }
        },
        "_links": {
          "actor": {
            "href": "/actors/2"
          },
          "movie": {
            "href": "/movies/1"
          },
          "self": {
            "href": "/roles/2"
          }
        },
        "name": "Jules Winnfield"
      }
    ]
  },
  "_links": {
    "roles": [
      {
        "href": "/roles/1"
      },
      {
        "href": "/roles/2"
      }
    ],
    "self": {
      "href": "/movies/1"
    }
  },
  "title": "Pulp Fiction",
  "year": 1994
}
```
