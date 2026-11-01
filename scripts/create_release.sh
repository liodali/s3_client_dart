#!/bin/bash

# Script to create a new release
# Usage: ./scripts/create_release.sh 1.0.0 "Release description"

set -e

if [ -z "$1" ]; then
    echo "Error: Version number required"
    echo "Usage: ./scripts/create_release.sh 1.0.0 \"Release description\""
    exit 1
fi

VERSION=$1
DESCRIPTION=${2:-"Release version $VERSION"}

# Ensure version starts with 'v'
if [[ ! $VERSION =~ ^v ]]; then
    VERSION="v$VERSION"
fi

echo "Creating release $VERSION"
echo "Description: $DESCRIPTION"
echo ""


tag="s3-client-dart-$VERSION"

# Check if tag already exists
if git rev-parse "$tag" >/dev/null 2>&1; then
    echo "Error: Tag $tag already exists"
    exit 1
fi

# Check for uncommitted changes
if ! git diff-index --quiet HEAD --; then
    echo "Error: You have uncommitted changes"
    echo "Please commit or stash them before creating a release"
    exit 1
fi

# Update version in pubspec.yaml
VERSION_NUMBER=${VERSION#v}  # Remove 'v' prefix
echo "Updating pubspec.yaml to version $VERSION_NUMBER..."
sed -i.bak "s/^version: .*/version: $VERSION_NUMBER/" pubspec.yaml
rm pubspec.yaml.bak

# Commit version change
git add pubspec.yaml
git commit -m "release go ffi: Bump version to $VERSION_NUMBER"

# Create annotated tag
echo "Creating tag $tag..."
git tag -a "$tag" -m "$DESCRIPTION"

# Push changes and tag
echo "Pushing to origin..."
git push origin main
git push origin "$tag"

echo ""
echo "✅ Release $VERSION created successfully!"
echo ""
echo "GitHub Actions will now:"
echo "  1. Build native libraries for macOS, Linux, and Windows"
echo "  2. Create a GitHub release with the libraries attached"
echo ""
echo "Monitor progress at:"
echo "  https://github.com/liodali/s3_client_dart/actions"
echo ""
echo "Once complete, the release will be available at:"
echo "  https://github.com/liodali/s3_client_dart/releases/tag/$tag"
