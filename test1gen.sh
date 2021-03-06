#!/bin/bash
set -u

differ=0

for testfile in t/expected/*.txt
do
    name=$(basename -s .txt $testfile)
    ./unit_test.sh minigo $name
    if [[ $? -ne 0 ]];then
        differ=1
    fi
done

if [[ $differ -eq 0 ]];then
    echo "All tests passed"
else
    echo "FAILED"
    exit 1
fi

set -e
./testerror.sh

echo "All tests passed"
