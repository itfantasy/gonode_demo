package game

import (
	"errors"

	"github.com/itfantasy/gonode"
	"github.com/itfantasy/gonode/utils/stl"

	"github.com/itfantasy/gonode-toolkit/toolkit"
	"github.com/itfantasy/gonode-toolkit/toolkit/gen_room"

	"github.com/itfantasy/gonode-icloud/icloud/gunpeer"
	"github.com/itfantasy/gonode-icloud/icloud/gunpeer/retcode"
	"github.com/itfantasy/gonode-icloud/icloud/opcode"
	"github.com/itfantasy/gonode-icloud/icloud/opcode/actorparam"
	"github.com/itfantasy/gonode-icloud/icloud/opcode/cacheop"
	"github.com/itfantasy/gonode-icloud/icloud/opcode/errorcode"
	"github.com/itfantasy/gonode-icloud/icloud/opcode/evncode"
	"github.com/itfantasy/gonode-icloud/icloud/opcode/gameparam"
	"github.com/itfantasy/gonode-icloud/icloud/opcode/paramcode"
	//	"github.com/itfantasy/gonode-icloud/icloud/opcode/recvgroup"
)

func HandleConn(id string) {
	gen_room.AddPeer(gen_room.NewRoomPeer(id))
}

func HandleMsg(id string, msg []byte) {

	gonode.Debug(msg)
	opCode, datas, err := gunpeer.ParseMsg(msg)
	if err != nil {
		gonode.LogError(err)
		return
	}
	peer, ok := gen_room.GetPeer(id)
	if !ok {
		gonode.LogError(errors.New("peer missing..." + id))
		return
	}
	switch opCode {
	case opcode.Authenticate:
		handleAuthenticate(peer, opCode, datas)
		break
	case opcode.CreateGame:
		handleCreateGame(peer, opCode, datas)
		break
	case opcode.JoinGame:
		handleJoinGame(peer, opCode, datas)
		break
	case opcode.RaiseEvent:
		handleRaiseEvent(peer, opCode, datas)
		break
	case opcode.SetProperties:
		handleSetProperties(peer, opCode, datas)
	default:
		break
	}
}

func HandleClose(id string) {
	peer, ok := gen_room.GetPeer(id)
	if !ok {
		return
	}
	room, actor, err := gen_room.GetActorInRoom(peer.PeerId(), peer.RoomId())
	if err != nil {
		handleError(peer, 0, err)
		return
	}
	pubLeaveEvent(peer, actor, room)
	_, _, err2 := gen_room.LeaveRoom(peer.PeerId(), room.RoomId())
	if err2 != nil {
		handleError(peer, 0, err2)
		return
	}
	if room.IsEmpty() {
		gen_room.DisposeRoom(room.RoomId())
	}
	gen_room.RemovePeer(id)
}

func handleSetProperties(peer *gen_room.RoomPeer, opCode byte, datas *gunpeer.PeerDatas) {
	gunpeer.SendResponse(peer.PeerId(), errorcode.Ok, opCode, nil)
}

func handleAuthenticate(peer *gen_room.RoomPeer, opCode byte, datas *gunpeer.PeerDatas) {
	gunpeer.SendResponse(peer.PeerId(), errorcode.Ok, opCode, nil)
}

func handleCreateGame(peer *gen_room.RoomPeer, opCode byte, datas *gunpeer.PeerDatas) {
	roomId, _ := datas.GetString(paramcode.GameId)
	if datas.Err() != nil {
		handleError(peer, opCode, datas.Err())
		return
	}

	room, actor, err := gen_room.CreateRoom(peer.PeerId(), roomId, toolkit.DEFAULT_LOBBY, 4) // TODO 4为临时逻辑, 默认大厅为临时逻辑
	if err != nil {
		handleError(peer, opCode, err)
	}
	peer.SetRoomId(room.RoomId())

	gunpeer.SendResponse(peer.PeerId(), errorcode.Ok, opCode, map[byte]interface{}{
		paramcode.ActorNr:        actor.ActorNr(),
		paramcode.GameProperties: RoomToHash(room),
		paramcode.Actors:         room.ActorsManager().GetAllActorNrs(),
	})
	pubJoinEvent(peer, actor, room)
}

func handleJoinGame(peer *gen_room.RoomPeer, opCode byte, datas *gunpeer.PeerDatas) {
	roomId, _ := datas.GetString(paramcode.GameId)
	if datas.Err() != nil {
		handleError(peer, opCode, datas.Err())
		return
	}

	room, actor, err := gen_room.JoinRoom(peer.PeerId(), roomId)
	if err != nil {
		handleError(peer, opCode, retcode.Err_NoMatchFound)
		return
	}
	peer.SetRoomId(room.RoomId())

	gunpeer.SendResponse(peer.PeerId(), errorcode.Ok, opCode, map[byte]interface{}{
		paramcode.ActorNr:        actor.ActorNr(),
		paramcode.GameProperties: RoomToHash(room),
		paramcode.Actors:         room.ActorsManager().GetAllActorNrs(),
	})

	pubJoinEvent(peer, actor, room)
}

func handleRaiseEvent(peer *gen_room.RoomPeer, opCode byte, datas *gunpeer.PeerDatas) {
	// send self resp
	gunpeer.SendResponse(peer.PeerId(), errorcode.Ok, opCode, nil)

	eventCode, _ := datas.GetByte(paramcode.Code)
	recvGroup, _ := datas.GetByte(paramcode.ReceiverGroup)
	cacheOp, _ := datas.GetByte(paramcode.Cache)
	//eventData, _ := datas.GetByte(paramcode.Data)
	if datas.Err() != nil {
		handleError(peer, opCode, datas.Err())
		return
	}

	_, actor, err := gen_room.GetActorInRoom(peer.PeerId(), peer.RoomId())
	if err != nil {
		handleError(peer, opCode, err)
	}

	evnDatas, err := gunpeer.EventDatas(eventCode, map[byte]interface{}{
		paramcode.ActorNr: actor.ActorNr(),
		paramcode.Code:    byte(eventCode),
		paramcode.Data:    datas.RawBytes()[5:],
	})
	if err != nil {
		handleError(peer, opCode, err)
	}

	addToRoomCache := (cacheOp == cacheop.AddToRoomCache || cacheOp == cacheop.AddToRoomCacheGlobal)
	gen_room.RaiseEvent(peer.PeerId(), peer.RoomId(), evnDatas, recvGroup, addToRoomCache)
}

func pubJoinEvent(peer *gen_room.RoomPeer, actor *gen_room.Actor, room *gen_room.RoomEntity) {
	gen_room.RcvCacheEvent(peer.PeerId(), room.RoomId())

	hashTable := stl.NewHashTable()
	hashTable.Set(actorparam.Nickname, "")

	evnDatas, err := gunpeer.EventDatas(evncode.Join, map[byte]interface{}{
		paramcode.ActorProperties: hashTable.Raw(),
		paramcode.ActorNr:         actor.ActorNr(),
		paramcode.Actors:          room.ActorsManager().GetAllActorNrs(),
	})
	if err != nil {
		handleError(peer, evncode.Join, err)
	}

	gen_room.RaiseEvent(peer.PeerId(), room.RoomId(), evnDatas, gen_room.RcvGroup_All, true)
}

func pubLeaveEvent(peer *gen_room.RoomPeer, actor *gen_room.Actor, room *gen_room.RoomEntity) {
	evnDatas, err := gunpeer.EventDatas(evncode.Leave, map[byte]interface{}{
		paramcode.ActorNr:    actor.ActorNr(),
		paramcode.Actors:     room.ActorsManager().GetAllActorNrs(),
		paramcode.IsInactive: false,
	})
	if err != nil {
		handleError(peer, evncode.Leave, err)
	}

	gen_room.RaiseEvent(peer.PeerId(), room.RoomId(), evnDatas, gen_room.RcvGroup_Others, false)
}

func handleError(peer *gen_room.RoomPeer, opCode byte, err error) {
	gonode.LogError(err, gonode.LogSource(1))
}

func RoomToHash(room *gen_room.RoomEntity) map[interface{}]interface{} {
	hash := make(map[interface{}]interface{})
	list := make([]interface{}, 0, 0)
	hash[gameparam.LobbyProperties] = list
	hash[gameparam.CleanupCacheOnLeave] = true
	hash[gameparam.MaxPlayers] = room.MaxPeers()
	hash[gameparam.IsVisible] = true
	hash[gameparam.IsOpen] = true
	hash[gameparam.MasterClientId] = room.MasterId()
	return hash
}
