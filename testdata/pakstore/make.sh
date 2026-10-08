#!/bin/sh
# Regenerates the Pak Store fixtures. The schema is the one on a real device
# (tg5040, 2026-09-28). Needs the sqlite3 CLI; the .db files are committed so
# tests do not.
set -eu
cd "$(dirname "$0")"
rm -f ./*.db

schema='CREATE TABLE installed_paks
(
    name          text not null,
    display_name  text not null,
    pak_id        text,
    repo_url      text,
    type          text not null,
    version       text not null,
    can_uninstall int  not null,
    unique (name)
);'
itch="('Itch-io','Itch-io','3JM2zY4UKh','https://github.com/carroarmato0/NextUI-Itchio-Pak','TOOL'"

sqlite3 single.db "$schema" "INSERT INTO installed_paks VALUES
  ('Pak Store','Pak Store','xK9mR2vL4w','https://github.com/LoveRetro/nextui-pak-store','TOOL','v4.2.0',0),
  $itch,'v1.0.23',1),
  ('BMO','BMO','qYyqrS1Jfz','https://github.com/example/bmo','TOOL','v1.0.1',1);"

# Matched by name when pak_id is missing.
sqlite3 byname.db "$schema" "INSERT INTO installed_paks VALUES ('Itch-io','Itch-io',NULL,NULL,'TOOL','v1.1.0-rc2',1);"

# 512-byte pages and 400 rows: a table b-tree with interior pages, two levels deep.
sqlite3 multipage.db "PRAGMA page_size=512;" "$schema" \
  "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<400)
   INSERT INTO installed_paks SELECT printf('Pak %03d',i), printf('Pak number %03d',i),
     printf('id%08d',i), printf('https://github.com/example/pak-%03d',i), 'TOOL', 'v0.1.0', 1 FROM n;" \
  "INSERT INTO installed_paks VALUES $itch,'v1.0.25',1);"

sqlite3 empty.db "$schema"
sqlite3 notable.db "CREATE TABLE other (x text);"

# A row too large for its page spills into overflow pages, which the reader
# does not follow.
sqlite3 overflow.db "$schema" "$(printf "INSERT INTO installed_paks VALUES %s,'v1.0.23',1);" "$itch")" \
  "INSERT INTO installed_paks VALUES ('Big', substr(replace(hex(zeroblob(2500)),'0','x'),1,5000), NULL, NULL, 'TOOL', 'v1', 1);"

head -c 6000 single.db > truncated.db
printf 'not a database at all' > badheader.db

echo "fixtures written: $(ls ./*.db | tr '\n' ' ')"
