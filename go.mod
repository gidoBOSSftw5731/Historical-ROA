module github.com/gidoBOSSftw5731/Historical-ROA

go 1.13

require (
	cloud.google.com/go/datastore v1.2.0
	github.com/gidoBOSSftw5731/Historical-ROA/proto v0.0.0-00010101000000-000000000000
	github.com/gidoBOSSftw5731/log v0.0.0-20190718204308-3ae037c6203f
	github.com/lib/pq v1.7.0
	google.golang.org/protobuf v1.25.0
)

replace github.com/gidoBOSSftw5731/Historical-ROA/proto => ./proto
