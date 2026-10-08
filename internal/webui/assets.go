package webui

import _ "embed"

//go:embed index.html
var Index []byte

//go:embed app.js
var AppJS []byte

//go:embed style.css
var StyleCSS []byte
