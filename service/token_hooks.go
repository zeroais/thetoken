package service

// TOKEN fork hook (tokenops/metrics): called for every upstream channel
// error, including errors that a retry later recovers from.
var OnChannelError func(channelID, statusCode int)
