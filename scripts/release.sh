#!/usr/bin/env bash

set -eou pipefail

# This script releases a new version of the ngrok-operator app and Helm chart

## Guards

# git tree must be clean
if `git status --porcelain | grep -q .`; then
  echo "ERROR: Working directory is not clean. Please commit all changes before releasing."
  exit 1
fi

## Helpers

color_echo () {
    local n_flag=""
	if [[ "$1" == "-n" ]]
	then
		n_flag="-n"
		shift
	fi
	is_stderr=false
	case $1 in
		(red) color="1;31"
			is_stderr=true  ;;
		(green) color="1;32"  ;;
		(yellow) color="1;33"  ;;
		(blue) color="1;34"  ;;
		(purple) color="1;35"  ;;
		(cyan) color="1;36"  ;;
		(white) color="0"  ;;
		(*) color=$1  ;;
	esac
	shift
	if [[ $is_stderr == true ]]
	then
		echo -e $n_flag "\033[${color}m$@\033[0m" >&2
	else
		echo -e $n_flag "\033[${color}m$@\033[0m"
	fi
}

## Main

# collect current versions
curr_app_version=$(cat VERSION)
curr_chart_version=$(cat helm/ngrok-operator/Chart.yaml | yq '.version' -r)

# print version info
color_echo green "== Current Release Info:"
color_echo green "==   App Version:   $curr_app_version"
color_echo green "==   Chart Version: $curr_chart_version"
color_echo green "" # formatting

# prompt for next versions
read -p "Supply next ngrok-operator-<version> (current: $curr_app_version): " next_app_version
read -p "Supply next Helm chart <version> (current: $curr_chart_version): " next_chart_version
echo "" # formatting

# use defaults if no new version supplied
next_app_version=${next_app_version:-$curr_app_version}
next_chart_version=${next_chart_version:-$curr_chart_version}

# create git branch
echo "Creating release branch..."
git fetch origin main
git checkout -b release-ngrok-operator-$next_app_version-helm-chart-$next_chart_version origin/main
echo "" # formatting

# update the version files
echo $next_app_version > VERSION
yq -Y -i ".version = \"$next_chart_version\"" helm/ngrok-operator/Chart.yaml
yq -Y -i ".appVersion = \"$next_app_version\"" helm/ngrok-operator/Chart.yaml

# update the Helm snapshots
echo "Updating Helm snapshots..."
make helm-update-snapshots helm-test
echo "" # formatting

# find and echo the PRs between $from..$to
function gather_prs () {
    from=$1
    to=$2

    if ! `git tag -l | grep -q $from` ; then
        color_echo red "ERROR: Tag $from not found" >&2
        exit 1
    fi

    # format: <commit-sha> <commit-msg> ()<pr-number>)
    git log --pretty=format:"%h %s" --merges --grep="Merge pull request" $1..$2 \
      | while read line ; do
            commit_sha=$(echo $line | awk '{print $1}') # first field
            commit_msg=$(echo $line | awk '{$1=$NF=""; print }' | sed 's/^\ *//') # 2nd through 2nd-to-last fields, trim leading whitespace
            pr_number=$(echo $line | awk '{print $NF}' | tr -d '()#') # last field, remove parens and hash
            echo "- $commit_msg by @<gh-user> <$(git show -s --format='%an' $commit_sha)> in [#${pr_number}](https://github.com/ngrok/ngrok-operator/pull/${pr_number})"
    done
}

curr_version_tag="ngrok-operator-$curr_app_version"
prs_since_last_release=$(gather_prs $curr_version_tag HEAD)

# adds a templated changelog entry to the changelog_path
function add_changelog_entry() {
    changelog_path=$1
    tag=$2
    from_version=$3
    to_version=$4

# template for new changelog entry
# Intentionally not indented
changelog_next_version=$(cat <<EOF
## $to_version
**Full Changelog**: https://github.com/ngrok/ngrok-operator/compare/$tag-$from_version...$tag-$to_version

<!-- remove empty sections -->
<!-- PRs since last release: -->
$prs_since_last_release

### Added

- Add some example feature by @<user> in [#PR](https://github.com/ngrok/ngrok-operator/pull/0000)

### Changed

- Bump image version to \`$to_version\`
- Update some feature by @<user> in [#PR](https://github.com/ngrok/ngrok-operator/pull/0000)

### Fixed

- Fixed some feature by @<user> in [#PR](https://github.com/ngrok/ngrok-operator/pull/0000)

### Removed

- Removed some feature by @<user> in [#PR](https://github.com/ngrok/ngrok-operator/pull/0000)

EOF
)

    inserted=false
    cat $changelog_path | while read ; do
        line="$REPLY"
        if ! $inserted && grep -q "^## $from_version\$" <<< $line ; then
            inserted=true
            echo "$changelog_next_version" >> $changelog_path.new
            echo "" >> $changelog_path.new # formatting
            echo "$line" >> $changelog_path.new
        else
            echo "$line" >> $changelog_path.new
        fi
    done

    mv -f $changelog_path.new $changelog_path
}

color_echo green "" # formatting
color_echo green "PRs since last release:"
if [ -z "$prs_since_last_release" ]; then
    color_echo yellow "No PRs found since last release"
    color_echo yellow "Are you sure you want to make a release?"
    read -p "Press Enter to continue..."
else
    color_echo green "$prs_since_last_release"
fi
color_echo green "" # formatting

# prompt to update the app CHANGELOG
app_changelog_path=CHANGELOG.md
if `grep -q "^## $next_app_version\$" $app_changelog_path` ; then
  color_echo yellow "WARN: App $app_changelog_path already contains an entry for version $next_app_version"
  echo "Skipping $app_changelog_path update..."
else
    echo -n "Please update the App"
    color_echo -n yellow " $app_changelog_path "
    echo -n "with the changes for next version"
    color_echo -n green " $next_app_version "
    echo "" # formatting
    echo "Save and close the file when complete"
    read -p "Press Enter to continue..."

    # add new changelog entry and open the file for editing
    add_changelog_entry $app_changelog_path ngrok-operator $curr_app_version $next_app_version
    $EDITOR $app_changelog_path
fi
echo "" # formatting
echo "" # formatting

# prompt to update the chart CHANGELOG
chart_changelog_path=helm/ngrok-operator/CHANGELOG.md
if `grep -q "^## $next_chart_version\$" $chart_changelog_path` ; then
  color_echo yellow "WARN: Helm Chart $chart_changelog_path already contains an entry for version $next_chart_version"
  echo "Skipping $chart_changelog_path update..."
else
    echo -n "Please update the Helm Chart"
    color_echo -n yellow " $chart_changelog_path "
    echo -n "with the changes for next version"
    color_echo -n green " $next_chart_version "
    echo "" # formatting
    echo "Save and close the file when complete"
    read -p "Press Enter to continue..."

    # add new changelog entry and open the file for editing
    add_changelog_entry $chart_changelog_path helm-chart $curr_chart_version $next_chart_version
    $EDITOR $chart_changelog_path
fi
echo "" # formatting
echo "" # formatting

# files are updated, commit and share diff
echo "Committing changes..."
git add VERSION helm/ngrok-operator/Chart.yaml $app_changelog_path $chart_changelog_path
git commit -m "Release ngrok-operator-$next_app_version helm-chart-$next_chart_version"
echo "" # formatting
echo "Please review the changes, then push the commit"
read -p "Press Enter to continue..."
echo "" # formatting
git show -p HEAD
echo "" # formatting

color_echo green "Done! Please push the changes and open a PR to merge the release."
exit 0
