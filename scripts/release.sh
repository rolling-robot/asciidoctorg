#!/usr/bin/env bash
# Creates a release tag whose tree holds the contents of the git submodules.
#
# Go module downloads (go get, go install, the module proxy) do not include
# submodule contents, so a module version built from a commit of main lacks the
# Ruby sources that package rubysrc embeds. A release is therefore a commit
# made outside main, whose parent is the HEAD of main and whose tree is HEAD's
# with every submodule replaced by the files of its checked-out commit and
# .gitmodules removed. Go module zips leave out symbolic links, so a symbolic
# link in a submodule to one of its files becomes a copy of that file.
#
#   scripts/release.sh v0.1.0      creates the annotated tag v0.1.0
#   scripts/release.sh -n v0.1.0   creates only the commit and prints its hash
#
# Nothing is pushed; push the tag with: git push origin <version>.
set -euo pipefail

dry_run=
if [[ ${1-} == -n ]]; then
	dry_run=1
	shift
fi
if [[ $# -ne 1 || ! $1 =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
	echo "usage: $0 [-n] vMAJOR.MINOR.PATCH[-PRERELEASE]" >&2
	exit 2
fi
version=$1

cd "$(git rev-parse --show-toplevel)"

if [[ -n $(git status --porcelain --untracked-files=no) ]]; then
	echo "release: the work tree has uncommitted changes" >&2
	exit 1
fi
if [[ -z $dry_run ]] && git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
	echo "release: tag $version already exists" >&2
	exit 1
fi

index=$(mktemp)
trap 'rm -f "$index"' EXIT
# The release tree is built in a temporary index; ix runs git on it.
ix() { GIT_INDEX_FILE=$index git "$@"; }
ix read-tree HEAD
ix rm -q --cached .gitmodules

# Each submodule must be checked out at the commit HEAD records, unmodified,
# so that its work tree files are the files of that commit.
git ls-tree -r HEAD | awk '$2 == "commit" { print $3, $4 }' | while read -r commit path; do
	if [[ $(git -C "$path" rev-parse HEAD 2>/dev/null) != "$commit" ]]; then
		echo "release: submodule $path is not checked out at $commit (run git submodule update --init)" >&2
		exit 1
	fi
	if [[ -n $(git -C "$path" status --porcelain) ]]; then
		echo "release: submodule $path has local changes" >&2
		exit 1
	fi
	ix rm -q --cached "$path"
	# Store the submodule's regular files as blobs of this repository and add
	# them to the index under the submodule's path.
	git -C "$path" ls-files -s -z | while IFS= read -r -d '' entry; do
		mode=${entry%% *}
		file=${entry#*$'\t'}
		src=$path/$file
		if [[ $mode == 120000 ]]; then
			# A symbolic link to a file of the submodule is stored as a copy
			# of that file; any other link is left out.
			src=$(realpath -e -- "$src" 2>/dev/null) || continue
			[[ -f $src && $src == "$(realpath "$path")"/* ]] || continue
			mode=100644
		fi
		[[ $mode == 100644 || $mode == 100755 ]] || continue
		blob=$(git hash-object -w --no-filters -- "$src")
		printf '%s %s\t%s\0' "$mode" "$blob" "$path/$file"
	done | ix update-index -z --index-info
done

tree=$(ix write-tree)
head=$(git rev-parse HEAD)
commit=$(git commit-tree "$tree" -p "$head" -m "Release $version

The tree of $(git rev-parse --short "$head") with the contents of its git
submodules, so that Go module downloads include the embedded Ruby sources.")

if [[ -n $dry_run ]]; then
	echo "$commit"
	exit 0
fi
git tag -a "$version" -m "asciidoctor-go $version" "$commit"
echo "tagged $version ($commit); push it with: git push origin $version"
