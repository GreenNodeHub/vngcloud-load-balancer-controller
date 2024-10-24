#!/bin/bash

echo "Repo URL: $1";
echo "Change charts: $2";
echo "Version: $3";

current_dir=$(pwd)
folders=$2
IFS=',' read -ra folder_array <<< "$folders"
for folder in "${folder_array[@]}"; do
  cd $folder
  rm -rf ./*.tgz
  echo "Processing chart: $folder"

  # check if version is provided
  if [ -z "$3" ]; then
    echo "Version is not provided. Using the default version."
    helm package .
  else
    echo "Version: $3"
    helm package . --version $3
  fi

  for file in ./*.tgz; do
    if [ -f "$file" ]; then
      echo "Pushing: $file"
      helm push "$file" "$1"
    fi
  done
  rm -rf ./*.tgz
  cd $current_dir
done

echo "Push process completed."