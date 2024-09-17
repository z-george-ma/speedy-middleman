package mongo

import (
	"context"
	"lib"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoRepo[T any] mongo.Collection

// NewMongoClient returns a mongo client
func NewMongoClient(ctx context.Context, uri string) (*mongo.Client, error) {
	return mongo.Connect(ctx, options.Client().ApplyURI(uri))
}

// NewMongoRepo returns a strongly typed mongo repo
func NewMongoRepo[T any](client *mongo.Client, db string, coll string) *MongoRepo[T] {
	return (*MongoRepo[T])(client.Database(db).Collection(coll))
}

// Get returns a document by id
//
// Examples:
//
//	repo := lib.NewMongoRepo[lib.M](client, "oddsmatrix", "BettingOffer")
//	ret, err := repo.Get(lib.Timeout(10), "206866647864328704")
func (mr *MongoRepo[T]) Get(ctx context.Context, id string) (ret T, err error) {
	coll := (*mongo.Collection)(mr)

	data := coll.FindOne(ctx, bson.M{"_id": id})
	err = data.Decode(&ret)
	return
}

// Find returns slice of documents based on filter
//
// Examples:
//
//	repo := lib.NewMongoRepo[lib.M](client, "oddsmatrix", "BettingOffer")
//	ret, err := repo.Find(lib.Timeout(10), lib.M{"_id": "206866647864328704"})
func (mr *MongoRepo[T]) Find(ctx context.Context, filter lib.M) (ret []T, err error) {
	coll := (*mongo.Collection)(mr)

	curr, err := coll.Find(ctx, filter)
	curr.All(ctx, &ret)
	return
}

// FindAndModify returns "AFTER" document based on filter, update & insert
// will additionally upsert if flagged to do so via the upsert param
//
// Examples:
//
//	repo := lib.NewMongoRepo[lib.M](client, "oddsmatrix", "BettingOffer")
//	ret, err := repo.FindAndModify(lib.Timeout(10), "206866647864328704", bson.M{"$set": {"name.value": "newNameValue"}}, true)
func (mr *MongoRepo[T]) FindAndModify(ctx context.Context, id string, mongoUpdate bson.M, upsert bool) (ret T, err error) {
	coll := (*mongo.Collection)(mr)
	updateOptions := options.FindOneAndUpdate()
	updateOptions.SetUpsert(upsert)
	updateOptions.SetReturnDocument(options.After)

	data := coll.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id},
		mongoUpdate,
		updateOptions,
	)

	err = data.Decode(&ret)
	return
}
