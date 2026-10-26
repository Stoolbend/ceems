# REST API v1

The contract is one OpenAPI 3.1 file, `internal/httpapi/openapi.json`. A running
server sends it at `GET /api/v1/openapi.json` (any signed-in caller). A test
(`internal/httpapi/openapi_test.go`) checks it against the route registry and
against the running server, so it cannot drift silently.

Integrators use the `pages` and `me` operations. The `admin` operations (users,
grants, keys, templates, bindings, runs, fastext groups, settings) work with a browser session only; an API
key gets 403 on them.

## Getting a key

An admin creates a key with a signed-in session (keys cannot create keys):

```
POST /api/v1/keys        {"name": "sheet sync", "readOnly": false}
POST /api/v1/keys/{id}/grants   {"lo": "100", "hi": "1FF", "access": "write"}
```

The token in the create response is shown once; only its hash is stored. A key
can do nothing until it has a grant, and a key marked `readOnly` can never write,
whatever its grants say.

## Calling the API

Send `Authorization: Bearer <token>`. With that header present the cookie is
ignored, so a bad token is 401 even if a session cookie is also sent.

- A page outside the key's grants, or outside 100 to 8FF, is 404 (the same as
  a page that does not exist). A page the key can read but not write is 403 on a
  write. `GET /api/v1/pages` lists only the pages the key may read.
- `GET /api/v1/pages/{mpp}` returns `etag`. Send it in `If-Match` on a write to
  refuse the write if the page changed since you read it (412). Without
  `If-Match`, `PUT` replaces the page unchecked. `DELETE` requires it (428 without).
- A generated page (one with a `binding`) cannot be edited or deleted: 409.
- Limits: 5 requests a second per key (burst 20) and 20 a second per client
  address (burst 40) across the API, then 429. Page files and JSON bodies are at
  most 256 KiB (413). Server timeouts: 10 s for headers, 30 s to read a request,
  60 s to write a response.
- Client address: the connection's address, unless it is listed in the
  config key `trustedProxies` (addresses or CIDR prefixes). From a listed
  proxy the address is the rightmost `X-Forwarded-For` entry that is not itself
  listed; from anywhere else the header is ignored. The limits, the audit
  `remote` and the request log all use it. List only the proxy's own address or
  subnet: a listed address can impersonate any client IP.
- Every write is audited with the actor `key:<id>`, and every request is logged
  with the route pattern, status and actor.

## Example

Read a page, then replace it from its rows document or from a `.tti` file,
each guarded by an etag:

```
TOKEN=...                # the key's token
BASE=http://localhost:8080/api/v1

# read the page document and its etag
curl -s -H "Authorization: Bearer $TOKEN" $BASE/pages/100 > page.json
ETAG=$(sed -n 's/.*"etag":"\([0-9a-f]*\)".*/\1/p' page.json)

# replace the rows document (edit page.json first)
curl -s -X PUT -H "Authorization: Bearer $TOKEN" -H "If-Match: $ETAG" \
     --data-binary @page.json $BASE/pages/100

# or replace the page from a .tti file (re-read the etag first if you wrote above)
curl -s -X PUT -H "Authorization: Bearer $TOKEN" -H "If-Match: $ETAG" \
     --data-binary @P100.tti $BASE/pages/100/tti
```

Each successful write changes the page, so its etag changes too: the response
carries the new one, and a stale `If-Match` is refused with 412.

## Fastext groups and the index link

Admin only. A fastext group sets the four coloured links (red, green, yellow, cyan)
of its member pages, and a paged group also links each member to the previous and
next one. A group also sets the text of each coloured link (`texts`, four strings of up to
9 cells, optional) and writes row 24 on every subpage of its members: four 10-cell fields,
each a colour code then its text padded to 9. Prev and Next carry the fixed ` [ Prev  ` and
`  Next ] ` and are blank at the ends of the group. A page that leaves a group keeps its
links and gets an empty row 24. A member page's row 24 from the editor is overwritten. Routes: `GET` and `POST /fastext-groups`, `GET`, `PUT` and `DELETE`
`/fastext-groups/{id}`, and `PUT /fastext-groups/{id}/pages` with `{"pages": ["100", ...]}`,
which sets the whole membership in order. A page already in another group is 409
`in_group`. A page document carries a read-only `fastextGroup` (`{id, name, paged}` or `null`) and `canWrite`, whether the caller may write the page (ignored on PUT).

`PUT /settings` requires `indexPageEnabled` and `indexPage` (three hex digits, magazine 1
to 8, not `8ff`); leaving one out is 400 and changes nothing. With the index on, every
page's sixth link is `indexPage`. For a user session, `me` and the login response carry
`indexPage` (the page when the index is on, else `null`).
