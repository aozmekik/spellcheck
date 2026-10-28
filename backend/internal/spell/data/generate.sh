#!/bin/sh
# Rebuilds words.txt: the ESDB (formerly SCOWL) size-60 US English word list
# plus the few words in extra.txt, with a usage count for each word from Peter
# Norvig's count_1w.txt. The output is committed, so this only needs to run to
# update the list. See NOTICE.
set -eu
cd "$(dirname "$0")"

esdb_url='https://app.aspell.net/create?max_size=60&spelling=US&variant_level=1&diacritic=both&special=hacker&download=wordlist&encoding=utf-8&format=inline'
counts_url='https://norvig.com/ngrams/count_1w.txt'
counts_sha256='51df159fd3de12b20e403c108f526e96dbd723d9cabdd5f17955cdc16059e690'
# The ESDB generator always serves the latest database, so pin the revision
# words.txt was built from and stop if it has moved on.
esdb_revision='1e5b7d3'

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -fsSL "$esdb_url" -o "$tmp/esdb.txt"
curl -fsSL "$counts_url" -o "$tmp/counts.txt"
echo "$counts_sha256  $tmp/counts.txt" | shasum -a 256 -c - >/dev/null

if ! grep -qx -- '---' "$tmp/esdb.txt"; then
	echo "generate.sh: the ESDB download has no '---' line; its format changed" >&2
	exit 1
fi
if ! grep -q "^ESDB Git Revision: .*\[$esdb_revision\]" "$tmp/esdb.txt"; then
	echo "generate.sh: ESDB is no longer at revision $esdb_revision:" >&2
	grep '^ESDB Git Revision' "$tmp/esdb.txt" >&2
	echo "review what changed, then update esdb_revision here and in NOTICE" >&2
	exit 1
fi

# Keep the words after the header, minus single letters other than "a" and
# "I", then the words in extra.txt. A word with no count of its own, like a
# contraction, borrows the count of its spelling without apostrophes: the
# corpus dropped them, so its "dont" counts "don't". Anything else gets 1.
awk -F '\t' -v q="'" '
	NR == FNR { count[$1] = $2; next }
	FILENAME == "extra.txt" { body = 1 }
	!body { body = ($0 == "---"); next }
	$0 == "" || /^#/ || (length($0) == 1 && $0 != "a" && $0 != "I") { next }
	{
		key = tolower($0)
		bare = key
		gsub(q, "", bare)
		n = (key in count) ? count[key] : ((bare in count) ? count[bare] : 1)
		print $0 "\t" n
	}
' "$tmp/counts.txt" "$tmp/esdb.txt" extra.txt > words.txt

lines=$(wc -l < words.txt | tr -d ' ')
if [ "$lines" -lt 100000 ] || [ "$lines" -gt 115000 ]; then
	echo "generate.sh: got $lines words, expected 100k to 115k; check the sources" >&2
	exit 1
fi
echo "generate.sh: wrote $lines words to words.txt" >&2
