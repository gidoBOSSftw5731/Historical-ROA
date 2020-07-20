module github.com/gidoBOSSftw5731/Historical-ROA

go 1.13

require (
	cloud.google.com/go v0.61.0 // indirect
	cloud.google.com/go/bigquery v1.9.0 // indirect
	cloud.google.com/go/datastore v1.2.0
	github.com/gidoBOSSftw5731/Historical-ROA/movefromoldtonew v0.0.0-20200718152542-93a147c56a97
	github.com/gidoBOSSftw5731/Historical-ROA/proto v0.0.0-00010101000000-000000000000
	github.com/gidoBOSSftw5731/log v0.0.0-20190718204308-3ae037c6203f
	github.com/jackc/pgservicefile v0.0.0-20200714003250-2b9c44734f2b // indirect
	github.com/jackc/pgx v3.6.2+incompatible
	github.com/jackc/pgx/v4 v4.7.1
	github.com/lib/pq v1.7.0
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae // indirect
	golang.org/x/tools v0.0.0-20200717024301-6ddee64345a6 // indirect
	google.golang.org/genproto v0.0.0-20200715011427-11fb19a81f2c // indirect
	google.golang.org/protobuf v1.25.0
)

replace github.com/gidoBOSSftw5731/Historical-ROA/proto => ./proto
