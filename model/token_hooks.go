package model

// TOKEN fork hook (tokenops/metrics): called for every billed request,
// whether or not consume logs are stored. Nil when metrics are off.
var OnConsumeLog func(params RecordConsumeLogParams)
