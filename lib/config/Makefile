
all: test

test: test_setup test01 test02 test_passed

test_setup:

test_passed:
	echo PASS

test01:
	go test -v

test02:
	go test -cover

# Run all tests
# go test -v

# Run specific test
# go test -v -run TestSetDefaults_StringFields

# Check coverage
# go test -cover
