#!/bin/bash
# list_skills - List all available skills
# Usage: list_skills

if [ -z "$ANVILLM_SKILLS_PATH" ]; then
  echo "[]"
  exit 0
fi

skills=()
for dir in "$ANVILLM_SKILLS_PATH"/*; do
  if [ -d "$dir" ] && [ -f "$dir/SKILL.md" ]; then
    name=$(basename "$dir")
    desc=$(grep -m1 '^description:' "$dir/SKILL.md" | sed 's/^description: *//')
    skills+=("{\"name\":\"$name\",\"description\":\"$desc\"}")
  fi
done

echo "[$(IFS=,; echo "${skills[*]}")]"
