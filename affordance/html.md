# html
> skill: lark-html

## +publish
Publish a local HTML entry and its static assets as an HTML document. Reuse the same entry path to update it, or target an existing document with edit permission. Always use the returned token and link because a denied target falls back to the entry path publication.

### Avoid when
- Publishing a Miaoda app with an app_id: use apps +deploy.
- Downloading an existing document: use html +fetch.

### Examples
**Publish a local HTML page**
```bash
lark-cli html +publish --file-path ./page.html
```
**Update an existing HTML document**
```bash
lark-cli html +publish --file-path ./page.html --token <html_token>
```
**Preview the file manifest**
```bash
lark-cli html +publish --file-path ./page.html --dry-run
```

### Skills
- lark-html/references/lark-html-publish.md

## +fetch
Download an HTML document's source ZIP using its token.

### Avoid when
- Downloading a Miaoda app: use apps +export.
- Opening the public page: the source archive requires separate download permission.

### Examples
**Save the source archive to a file**
```bash
lark-cli html +fetch --token <token> --output ./downloads/src.zip
```

### Skills
- lark-html/references/lark-html-fetch.md
