// Package builtin registers the built-in app plugins with the default
// registry via side-effect imports. Adding an app = one new plugin
// package + one import line here.
package builtin

import (
	_ "github.com/rpuneet/mycel/pkg/gateway/rss"
	_ "github.com/rpuneet/mycel/pkg/gateway/slack"
	_ "github.com/rpuneet/mycel/pkg/gateway/telegram"
	_ "github.com/rpuneet/mycel/pkg/gateway/webhook"
	_ "github.com/rpuneet/mycel/pkg/gateway/whatsapp"
)
