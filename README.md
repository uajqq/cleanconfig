# cleanconfig

`cleanconfig` is a dependency-free Go formatter for WeeWX-style `.conf` files, and other INI-type config files.
It is ConfigObj-aware:

- section depth is derived from `[section]`, `[[subsection]]`, and deeper
  ConfigObj section markers
- assignments, quoted keys, quoted values, lists, and inline `#` comments are
  split only when the delimiter is outside quotes
- comments and blank lines are preserved
- multiline quoted values are preserved instead of rewritten

## Build

```sh
go build ./cmd/cleanconfig
```

Install into your Go bin directory:

```sh
go install ./cmd/cleanconfig
```

Run from the checkout without installing:

```sh
go run ./cmd/cleanconfig --help
```

## Usage

Format stdin to stdout:

```sh
cleanconfig < charts.conf > charts.conf.formatted
```

Rewrite files in place:

```sh
cleanconfig --write charts.conf
```

Align equals signs in contiguous assignment blocks:

```sh
cleanconfig --align-equals --write charts.conf
```

Check whether files are already formatted:

```sh
cleanconfig --check charts.conf
```

Preview a patch:

```sh
cleanconfig --diff --align-equals charts.conf
```

By default, existing quoted values are normalized to double quotes and unquoted
ConfigObj values stay unquoted. Use `--quote-style preserve` to leave quote
marks alone, `--quote-style single` for single quotes, or `--quote-style auto`
to prefer double quotes unless single quotes avoid escaping embedded double
quotes.

Use `--quote-policy all` only when you deliberately want every scalar/list item
quoted.

## Neovim

With `formatprg`:

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = { "conf", "dosini" },
  callback = function()
    vim.bo.formatprg = "cleanconfig --stdin-filepath % -"
  end,
})
```

With `conform.nvim`:

```lua
require("conform").setup({
  formatters = {
    cleanconfig = {
      command = "cleanconfig",
      args = { "--stdin-filepath", "$FILENAME", "-" },
      stdin = true,
    },
  },
  formatters_by_ft = {
    conf = { "cleanconfig" },
    dosini = { "cleanconfig" },
  },
})
```

To enable alignment from Neovim, add `"--align-equals"` to the args list.

A small `formatprg` helper is also available at
`contrib/nvim/cleanconfig.lua` if you prefer to vendor the setup in your Neovim
config.
