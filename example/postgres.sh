# requires env vars:
# 	POSTGRES_DATA_DIR=/path/to/data/directory
# usage: sh postgres.sh [bridge network name] [container name] [local port]
# ex: sh postgres.sh example app.db.example.tinkeractive.com 5432

docker container kill $2
docker system prune -f --volumes
docker pull postgres:15
# create a docker network for container communication
docker network create $1
# run the postgres container
# modify to optionally mount test/init data volume to container
docker run -d --net $1 --name $2 -p $3:5432 \
	-e POSTGRES_HOST_AUTH_METHOD=trust \
	-v $POSTGRES_DATA_DIR:/container/dat \
	postgres:15
# install the psql postgres client
sudo sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo apt-key add -
sudo apt-get update
sudo apt-get -y install postgresql-client-14 
# initialize the database
psql -h localhost -p $3 -U postgres \
	-c "\i schema/app/public.sql"
