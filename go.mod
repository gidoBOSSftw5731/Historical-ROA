module github.com/gidoBOSSftw5731/Historical-ROA

go 1.13

require (
	cloud.google.com/go v0.61.0 // indirect
	cloud.google.com/go/bigquery v1.9.0
	cloud.google.com/go/datastore v1.2.0
	github.com/blend/go-sdk v2.0.0+incompatible
	github.com/gidoBOSSftw5731/Historical-ROA/movefromoldtonew v0.0.0-20200718152542-93a147c56a97
	github.com/gidoBOSSftw5731/Historical-ROA/proto v0.0.0-00010101000000-000000000000
	github.com/gidoBOSSftw5731/log v0.0.0-20190718204308-3ae037c6203f
	github.com/jackc/pgservicefile v0.0.0-20200714003250-2b9c44734f2b // indirect
	github.com/jackc/pgx v3.6.2+incompatible
	github.com/jackc/pgx/v4 v4.7.1
	github.com/lib/pq v1.7.0
	github.com/m-zajac/json2go v1.0.2 // indirect
	github.com/mohae/firkin v0.0.0-20160628234414-44b753e65e9f // indirect
	github.com/mohae/json2go v0.0.0-20161213021912-4e6509b62191 // indirect
	github.com/shomali11/util v0.0.0-20200329021417-91c54758c87b
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae // indirect
	golang.org/x/tools v0.0.0-20200717024301-6ddee64345a6 // indirect
	google.golang.org/api v0.29.0
	google.golang.org/genproto v0.0.0-20200715011427-11fb19a81f2c // indirect
	google.golang.org/protobuf v1.25.0
)

replace github.com/gidoBOSSftw5731/Historical-ROA/proto => ./proto
