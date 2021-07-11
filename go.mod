module github.com/gidoBOSSftw5731/Historical-ROA

go 1.13

require (
	cloud.google.com/go v0.86.0 // indirect
	cloud.google.com/go/bigquery v1.19.0
	cloud.google.com/go/datastore v1.2.0
	github.com/blend/go-sdk v2.0.0+incompatible
	github.com/gidoBOSSftw5731/Historical-ROA/movefromoldtonew v0.0.0-20200718152542-93a147c56a97
	github.com/gidoBOSSftw5731/Historical-ROA/proto v0.0.0-20210702005558-8adba536b954
	github.com/gidoBOSSftw5731/log v0.0.0-20210527210830-1611311b4b64
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/jackc/pgservicefile v0.0.0-20200714003250-2b9c44734f2b // indirect
	github.com/jackc/pgx v3.6.2+incompatible
	github.com/jackc/pgx/v4 v4.7.1
	github.com/lib/pq v1.7.0
	github.com/m-zajac/json2go v1.0.2 // indirect
	github.com/mohae/firkin v0.0.0-20160628234414-44b753e65e9f // indirect
	github.com/mohae/json2go v0.0.0-20161213021912-4e6509b62191 // indirect
	github.com/shomali11/util v0.0.0-20200329021417-91c54758c87b
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e // indirect
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
	google.golang.org/api v0.50.0
	google.golang.org/genproto v0.0.0-20210708141623-e76da96a951f // indirect
	google.golang.org/protobuf v1.27.1
)

replace github.com/gidoBOSSftw5731/Historical-ROA/proto => ./proto
