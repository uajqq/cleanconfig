local M = {}

local defaults = {
  command = "cleanconfig",
  align_equals = false,
  filetypes = { "conf", "dosini" },
}

function M.setup(opts)
  opts = vim.tbl_extend("force", defaults, opts or {})

  vim.api.nvim_create_autocmd("FileType", {
    pattern = opts.filetypes,
    callback = function()
      local args = { opts.command, "--stdin-filepath", vim.fn.expand("%:p"), "-" }
      if opts.align_equals then
        table.insert(args, 2, "--align-equals")
      end
      vim.bo.formatprg = table.concat(vim.tbl_map(vim.fn.shellescape, args), " ")
    end,
  })
end

return M
