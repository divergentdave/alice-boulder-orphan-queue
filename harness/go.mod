module github.com/divergentdave/alice-boulder-orphan-queue/harness

go 1.19

require github.com/beeker1121/goque v1.0.3-0.20191103205551-d618510128af

require (
	github.com/golang/snappy v0.0.0-20180518054509-2e65f85255db // indirect
	github.com/syndtr/goleveldb v1.0.0 // indirect
)

replace github.com/syndtr/goleveldb v1.0.0 => github.com/divergentdave/goleveldb v0.0.0-20230205162341-d26ce9d23dad

replace github.com/beeker1121/goque v1.0.3-0.20191103205551-d618510128af => github.com/divergentdave/goque v0.0.0-20230205171309-2a442e663681
