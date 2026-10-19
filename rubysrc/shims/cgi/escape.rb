# frozen_string_literal: true

# rbgo provides CGI.escapeHTML et al. through the cgi library itself, but has
# no separate cgi/escape feature; load the full library to satisfy the require.
require 'cgi'
