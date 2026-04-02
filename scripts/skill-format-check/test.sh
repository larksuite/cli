#!/bin/bash

# Get the directory of this script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
INDEX_JS="$DIR/index.js"
TEMP_DIR="$DIR/tests/temp_test_dir"

echo "=== Running tests for skill-format-check ==="
echo "Index script: $INDEX_JS"

prepare_fixture() {
    local test_name=$1
    rm -rf "$TEMP_DIR"
    mkdir -p "$TEMP_DIR"
    if [ ! -d "$DIR/tests/$test_name" ]; then
        echo "❌ Missing fixture directory: $DIR/tests/$test_name"
        exit 1
    fi
    cp -r "$DIR/tests/$test_name" "$TEMP_DIR/" || {
        echo "❌ Failed to copy fixture: $test_name"
        exit 1
    }
}

# Function to run a positive test
run_positive_test() {
    local test_name=$1
    echo -e "\n--- [Positive] $test_name ---"
    
    prepare_fixture "$test_name"
    
    node "$INDEX_JS" "$TEMP_DIR"
    
    if [ $? -eq 0 ]; then
        echo "✅ Passed! (Correctly validated $test_name)"
        rm -rf "$TEMP_DIR"
        return 0
    else
        echo "❌ Failed! Expected $test_name to pass but it failed."
        rm -rf "$TEMP_DIR"
        exit 1
    fi
}

# Function to run a negative test
run_negative_test() {
    local test_name=$1
    echo -e "\n--- [Negative] $test_name ---"
    
    prepare_fixture "$test_name"
    
    # Capture output for diagnostics while still treating non-zero as expected
    local log_file="$TEMP_DIR/.validator.log"
    node "$INDEX_JS" "$TEMP_DIR" > "$log_file" 2>&1
    local exit_code=$?
    
    if [ $exit_code -ne 0 ]; then
        echo "✅ Passed! (Correctly rejected $test_name)"
        rm -rf "$TEMP_DIR"
        return 0
    else
        echo "❌ Failed! Expected $test_name to fail but it passed."
        if [ -s "$log_file" ]; then
            echo "--- Validator output ---"
            cat "$log_file"
        fi
        rm -rf "$TEMP_DIR"
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
