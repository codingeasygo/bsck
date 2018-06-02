//Package bsck provider tcp socket proxy router
//
//the supported router is client->(slaver->master->slaver)*-server,
//
//the channel of slaver to master can be multi physical tcp connect by different router
package bsck

//ShowLog will show more log.
var ShowLog = 0
