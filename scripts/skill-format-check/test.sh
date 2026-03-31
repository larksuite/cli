#!/bin/bash

# Get the directory of this script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
INDEX_JS="$DIR/index.js"

echo "=== Running tests for skill-format-check ==="
echo "Index script: $INDEX_JS"

# Function to run a positive test
run_positive_test() {
    local test_name=$1
    echo -e "\n--- [Positive] $test_name ---"
    
    mkdir -p "$DIR/tests/temp_test_dir"
    cp -r "$DIR/tests/$test_name" "$DIR/tests/temp_test_dir/"
    
    node "$INDEX_JS" "$DIR/tests/temp_test_dir"
    
    if [ $? -eq 0 ]; then
        echo "✅ Passed! (Correctly validated $test_name)"
        rm -rf "$DIR/tests/temp_test_dir"
        return 0
    else
        echo "❌ Failed! Expected $test_name to pass but it failed."
        rm -rf "$DIR/tests/temp_test_dir"
        exit 1
    fi
}

# Function to run a negative test
run_negative_test() {
    local test_name=$1
    echo -e "\n--- [Negative] $test_name ---"
    
    mkdir -p "$DIR/tests/temp_test_dir"
    cp -r "$DIR/tests/$test_name" "$DIR/tests/temp_test_dir/"
    
    # Run the script and suppress error output since we expect it to fail
    node "$INDEX_JS" "$DIR/tests/temp_test_dir" > /dev/null 2>&1
    local exit_code=$?
    
    if [ $exit_code -ne 0 ]; then
        echo "✅ Passed! (Correctly rejected $test_name)"
        rm -rf "$DIR/tests/temp_test_dir"
        return 0
    else
        echo "❌ Failed! Expected $test_name to fail but it passed."
        rm -rf "$DIR/tests/temp_test_dir"
        exit 1
    fi
}

# Run positive tests
run_positive_test "good-skill"
run_positive_test "good-skill-minimal"
run_positive_test "good-skill-complex"

# Run negative tests
run_negative_test "bad-skill"
run_negative_test "bad-skill-no-frontmatter"
run_negative_test "bad-skill-unclosed-frontmatter"

echo -e "\n🎉 All tests passed successfully!"
