#!/bin/sh
# Recebe no stdin: {"event","agent","tool","path","command","prompt","cwd"}.
# Sair com 2 nega a ação; a mensagem no stderr vai para o agente.
echo "Arquivos em .plan/ são editados só pela pessoa. Proponha a mudança em vez de aplicar." >&2
exit 2
