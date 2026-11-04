package bot

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// 1. MAIL SERVICE
// ============================================================================

// GetMailInbox fetches the current inbox list, unread count, and postage fee.
func (b *BotInstance) GetMailInbox() (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("bot session token unavailable; reconnect first")
	}

	charSlot := b.getCharSlot()
	payload := map[string]interface{}{
		"k":    "m_list",
		"char": charSlot,
	}

	res, err := b.executeClaimTransaction(token, payload)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// ReadMail marks an unread mail message as read.
func (b *BotInstance) ReadMail(mailID int) (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("bot session token unavailable")
	}

	charSlot := b.getCharSlot()
	payload := map[string]interface{}{
		"k":    "m_read",
		"id":   mailID,
		"char": charSlot,
	}

	return b.executeClaimTransaction(token, payload)
}

// CollectMailAttachment collects item and/or Penya attached to a mail message.
// Enforces inventory space check, COD penya deduction, character save, and /api/save flush.
func (b *BotInstance) CollectMailAttachment(mailID int) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("bot session token unavailable")
	}

	invList, _ := charData["inv"].([]interface{})
	if invList == nil {
		invList = make([]interface{}, 42)
		for i := range invList {
			invList[i] = map[string]interface{}{}
		}
	}

	charSlot := b.getCharSlot()
	payload := map[string]interface{}{
		"k":    "m_take",
		"id":   mailID,
		"char": charSlot,
	}

	res, err := b.executeClaimTransaction(token, payload)
	if err != nil {
		b.emitLog("action", fmt.Sprintf("Failed to collect mail #%d: %v", mailID, err), "warn")
		return nil, err
	}

	if okVal, exists := res["ok"].(bool); exists && !okVal {
		errMsg, _ := res["error"].(string)
		if errMsg == "" {
			errMsg = "Server rejected mail collection"
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	var collectedDesc []string

	// 1. Attached Item
	if itMap, ok := res["item"].(map[string]interface{}); ok && len(itMap) > 0 {
		if !insertStructuredItem(invList, itMap) {
			b.emitLog("action", "Warning: Bag was full, item dropped or could not fit into inventory", "warn")
		} else {
			idStr, _ := itMap["id"].(string)
			nVal := toInt(itMap["n"])
			if nVal <= 0 {
				nVal = 1
			}
			upVal := toInt(itMap["up"])
			if upVal > 0 {
				collectedDesc = append(collectedDesc, fmt.Sprintf("%s +%d (x%d)", idStr, upVal, nVal))
			} else {
				collectedDesc = append(collectedDesc, fmt.Sprintf("%s x%d", idStr, nVal))
			}
		}
	}

	// 2. Attached Penya
	if penya, ok := res["penya"].(float64); ok && penya > 0 {
		curPenya := b.getEffectivePenya(charData)
		b.setEffectivePenya(charData, curPenya+penya)
		collectedDesc = append(collectedDesc, fmt.Sprintf("%d Penya", int64(penya)))
	}

	// 3. Paid COD Penya
	if paid, ok := res["paid"].(float64); ok && paid > 0 {
		curPenya := b.getEffectivePenya(charData)
		b.setEffectivePenya(charData, curPenya-paid)
	}

	charData["inv"] = invList
	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	// Immediate /api/save flush matching scripts/mail.gd:137
	b.flushSave(true)

	if len(collectedDesc) > 0 {
		msg := fmt.Sprintf("Collected mail #%d: %s", mailID, strings.Join(collectedDesc, ", "))
		b.emitLog("action", msg, "info")
		b.emitChat("notice", "", msg, "#80ffaa")
	}

	return res, nil
}

// CollectAllMail loops through all mails with unclaimed items/Penya and collects them.
func (b *BotInstance) CollectAllMail() (map[string]interface{}, error) {
	inbox, err := b.GetMailInbox()
	if err != nil {
		return nil, err
	}

	mailList, ok := inbox["mail"].([]interface{})
	if !ok || len(mailList) == 0 {
		return map[string]interface{}{"collected": 0, "message": "Inbox is empty"}, nil
	}

	collectedCount := 0
	var errs []string

	// Reverse order collection matching scripts/mail.gd:147
	for i := len(mailList) - 1; i >= 0; i-- {
		m, ok := mailList[i].(map[string]interface{})
		if !ok {
			continue
		}
		hasItem := m["item"] != nil
		penya := toInt(m["penya"])
		if !hasItem && penya <= 0 {
			continue
		}

		mID := toInt(m["id"])
		if mID <= 0 {
			continue
		}

		// Check if bag has room for items
		if hasItem {
			charData, _, _, cErr := b.loadCharData()
			if cErr == nil {
				if invList, ok := charData["inv"].([]interface{}); ok {
					if countFreeInventorySlots(invList) < 1 {
						b.emitLog("action", "Bag is full - stopped mail collection.", "warn")
						break
					}
				}
			}
		}

		_, takeErr := b.CollectMailAttachment(mID)
		if takeErr != nil {
			errs = append(errs, fmt.Sprintf("#%d: %v", mID, takeErr))
		} else {
			collectedCount++
		}
	}

	summaryMsg := fmt.Sprintf("Collect All finished: %d letter(s) collected.", collectedCount)
	if len(errs) > 0 {
		summaryMsg += fmt.Sprintf(" Errors: %s", strings.Join(errs, "; "))
	}
	b.emitLog("action", summaryMsg, "info")

	return map[string]interface{}{
		"collected": collectedCount,
		"errors":    errs,
	}, nil
}

// DeleteMail permanently removes a mail message from the server.
func (b *BotInstance) DeleteMail(mailID int) (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("bot session token unavailable")
	}

	charSlot := b.getCharSlot()
	payload := map[string]interface{}{
		"k":    "m_delete",
		"id":   mailID,
		"char": charSlot,
	}

	res, err := b.executeClaimTransaction(token, payload)
	if err != nil {
		return nil, err
	}
	b.emitLog("action", fmt.Sprintf("Deleted mail #%d", mailID), "info")
	return res, nil
}

// SendMailFull sends mail with optional item, attached Penya, and optional Cash-On-Delivery (COD).
func (b *BotInstance) SendMailFull(to, title, text string, slotIndex, count, penya, cod int) (map[string]interface{}, error) {
	if to == "" {
		return nil, fmt.Errorf("recipient character name cannot be empty")
	}
	if title == "" {
		title = "Letter"
	}
	if penya < 0 {
		penya = 0
	}
	if cod < 0 {
		cod = 0
	}

	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	curPenya := b.getEffectivePenya(charData)
	postage := 500
	neededPenya := float64(penya + postage)
	if curPenya < neededPenya {
		return nil, fmt.Errorf("insufficient Penya (have %d, need %d including %d postage)",
			int64(curPenya), int64(neededPenya), postage)
	}

	var itemToSend interface{} = nil
	invList, _ := charData["inv"].([]interface{})

	if slotIndex >= 0 && slotIndex < len(invList) {
		slotMap, ok := invList[slotIndex].(map[string]interface{})
		if !ok || len(slotMap) == 0 {
			return nil, fmt.Errorf("selected inventory slot %d is empty", slotIndex)
		}
		if lk, _ := slotMap["lk"].(bool); lk {
			return nil, fmt.Errorf("selected item is locked")
		}
		if bd, _ := slotMap["bd"].(bool); bd {
			return nil, fmt.Errorf("selected item is bound and cannot be transferred")
		}

		avail := int(toFloat32(slotMap["n"]))
		if avail <= 0 {
			avail = 1
		}
		if count <= 0 || count > avail {
			count = avail
		}

		cloned := make(map[string]interface{})
		for k, v := range slotMap {
			cloned[k] = v
		}
		cloned["n"] = count
		itemToSend = cloned
	}

	if cod > 0 && (itemToSend == nil || penya > 0) {
		return nil, fmt.Errorf("cash on delivery requires an attached item and 0 attached Penya")
	}

	charSlot := b.getCharSlot()
	payload := map[string]interface{}{
		"k":     "m_send",
		"to":    to,
		"title": title,
		"text":  text,
		"item":  itemToSend,
		"penya": penya,
		"cod":   cod,
		"char":  charSlot,
	}

	res, err := b.executeClaimTransaction(token, payload)
	if err != nil {
		b.emitLog("action", fmt.Sprintf("Failed to send mail to %s: %v", to, err), "error")
		return nil, fmt.Errorf("mail server error: %w", err)
	}

	actualPostage := postage
	if pVal, ok := res["postage"].(float64); ok && int(pVal) > 0 {
		actualPostage = int(pVal)
	}
	b.setEffectivePenya(charData, curPenya-float64(penya+actualPostage))

	if slotIndex >= 0 && slotIndex < len(invList) {
		slotMap := invList[slotIndex].(map[string]interface{})
		avail := int(toFloat32(slotMap["n"]))
		if avail <= 0 {
			avail = 1
		}
		if count >= avail {
			invList[slotIndex] = map[string]interface{}{}
		} else {
			slotMap["n"] = avail - count
		}
	}

	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}
	b.flushSave(true)

	msg := fmt.Sprintf("Mail sent to '%s'! (Penya: %d, Postage: %d)", to, penya, actualPostage)
	b.emitLog("action", msg, "info")
	b.emitChat("system", "", msg, "#66ff99")

	return res, nil
}

// ============================================================================
// 2. BOUNTY, CONTRACTS & TRIAL SERVICE
// ============================================================================

// GetBountyContracts fetches the 3 daily level-appropriate monster hunting jobs.
func (b *BotInstance) GetBountyContracts() (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	charSlot := b.getCharSlot()
	return b.executeClaimTransaction(token, map[string]interface{}{
		"k":    "c_list",
		"char": charSlot,
	})
}

// ClaimBountyContract claims reward for contract index (0..2 for contracts, 3 for 3-job bonus).
func (b *BotInstance) ClaimBountyContract(index int) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	charSlot := b.getCharSlot()

	res, err := b.executeClaimTransaction(token, map[string]interface{}{
		"k":    "c_claim",
		"i":    index,
		"char": charSlot,
	})
	if err != nil {
		return nil, err
	}

	// If reward granted Penya or items, update local character
	if penya, ok := res["penya"].(float64); ok && penya > 0 {
		curPenya := b.getEffectivePenya(charData)
		b.setEffectivePenya(charData, curPenya+penya)
	}
	if itemsRaw, ok := res["items"].([]interface{}); ok {
		if invList, ok := charData["inv"].([]interface{}); ok {
			for _, itRaw := range itemsRaw {
				if itArr, ok := itRaw.([]interface{}); ok && len(itArr) >= 2 {
					idStr, _ := itArr[0].(string)
					cnt := int(toFloat32(itArr[1]))
					if idStr != "" {
						addItemToInventory(invList, idStr, cnt)
					}
				}
			}
		}
	}

	_ = b.persistCharData(charData, charFile, token)
	b.flushSave(true)

	b.emitLog("action", fmt.Sprintf("Claimed daily contract #%d reward!", index+1), "info")
	return res, nil
}

// GetBountyBoard fetches open player/monster bounties.
func (b *BotInstance) GetBountyBoard() (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	charSlot := b.getCharSlot()
	return b.executeClaimTransaction(token, map[string]interface{}{
		"k":    "b_list",
		"char": charSlot,
	})
}

// TakeBounty accepts a bounty by ID.
func (b *BotInstance) TakeBounty(bountyID int) (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	charSlot := b.getCharSlot()
	return b.executeClaimTransaction(token, map[string]interface{}{
		"k":    "b_take",
		"id":   bountyID,
		"char": charSlot,
	})
}

// DropBounty cancels an active bounty.
func (b *BotInstance) DropBounty(bountyID int) (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	charSlot := b.getCharSlot()
	return b.executeClaimTransaction(token, map[string]interface{}{
		"k":    "b_drop",
		"id":   bountyID,
		"char": charSlot,
	})
}

// TriggerDailyTrial starts a daily trial encounter where the bot is standing.
func (b *BotInstance) TriggerDailyTrial() (map[string]interface{}, error) {
	b.mu.Lock()
	if b.Status != "online" || b.ws == nil || !b.ws.IsConnected() {
		b.mu.Unlock()
		return nil, fmt.Errorf("bot is offline or not connected to game server")
	}
	b.mu.Unlock()

	pkt := map[string]interface{}{
		"t": "trial",
	}
	data, err := json.Marshal(pkt)
	if err != nil {
		return nil, err
	}

	if err := b.ws.Send(data); err != nil {
		return nil, fmt.Errorf("failed to send trial packet: %w", err)
	}

	b.emitLog("action", "Initiated Daily Trial challenge!", "info")
	return map[string]interface{}{"ok": true}, nil
}

// ============================================================================
// 3. REWARDS, BATTLE PASS & GACHA SERVICE
// ============================================================================

// GetBattlePass fetches the current Battle Pass status.
func (b *BotInstance) GetBattlePass() (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	charSlot := b.getCharSlot()
	return b.executeClaimTransaction(token, map[string]interface{}{
		"k":    "bpass",
		"char": charSlot,
	})
}

// PullGacha executes 1x or 10x Capsule Machine pulls.
func (b *BotInstance) PullGacha(wheelIndex int, count int) (map[string]interface{}, error) {
	if count != 1 && count != 10 {
		count = 1
	}

	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	charSlot := b.getCharSlot()
	payload := map[string]interface{}{
		"k":    "gacha",
		"w":    wheelIndex,
		"n":    count,
		"char": charSlot,
	}

	res, err := b.executeClaimTransaction(token, payload)
	if err != nil {
		return nil, err
	}

	if okVal, exists := res["ok"].(bool); exists && !okVal {
		errMsg, _ := res["error"].(string)
		return nil, fmt.Errorf("%s", errMsg)
	}

	// Insert prize items
	var wonItems []string
	if gotArr, ok := res["got"].([]interface{}); ok {
		if invList, ok := charData["inv"].([]interface{}); ok {
			for _, gRaw := range gotArr {
				if g, ok := gRaw.([]interface{}); ok && len(g) >= 2 {
					idStr, _ := g[0].(string)
					nVal := int(toFloat32(g[1]))
					if idStr != "" {
						addItemToInventory(invList, idStr, nVal)
						wonItems = append(wonItems, fmt.Sprintf("%s x%d", idStr, nVal))
					}
				}
			}
		}
	}

	_ = b.persistCharData(charData, charFile, token)
	b.flushSave(true)

	b.emitLog("action", fmt.Sprintf("Gacha pull x%d: %s", count, strings.Join(wonItems, ", ")), "info")
	return res, nil
}

// ============================================================================
// 4. SHARED ACCOUNT BANK & PIN SERVICE
// ============================================================================

// GetBankData loads the shared account bank items (42 slots) and Penya balance.
func (b *BotInstance) GetBankData() (map[string]interface{}, error) {
	b.mu.Lock()
	accKey := b.accountKey
	saveFiles := b.saveFiles
	hasPin := b.hasPin
	pinOk := b.pinOk
	b.mu.Unlock()

	bankFile := fmt.Sprintf("bank/%s.json", accKey)
	if rawStr, ok := saveFiles[bankFile]; ok && rawStr != "" {
		var d map[string]interface{}
		if err := json.Unmarshal([]byte(rawStr), &d); err == nil {
			d["ok"] = true
			d["hasPin"] = hasPin
			d["pinOk"] = pinOk
			return d, nil
		}
	}

	// Default empty 42-slot bank
	items := make([]interface{}, 42)
	for i := range items {
		items[i] = map[string]interface{}{}
	}
	return map[string]interface{}{
		"ok":     true,
		"hasPin": hasPin,
		"pinOk":  pinOk,
		"penya":  0,
		"items":  items,
	}, nil
}

// CheckBankPin verifies the character PIN with the server.
func (b *BotInstance) CheckBankPin(pin string) (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	cli := b.newGameHTTPClient()
	res, err := cli.CheckPin(token, pin)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "No PIN set yet") {
			b.mu.Lock()
			b.hasPin = false
			b.pinOk = true
			b.mu.Unlock()
			return map[string]interface{}{
				"ok":     true,
				"hasPin": false,
				"pinOk":  true,
				"noPin":  true,
			}, nil
		}
		return nil, err
	}

	if res != nil {
		if errMsg, ok := res["error"].(string); ok && errMsg != "" {
			if strings.Contains(errMsg, "No PIN set yet") {
				b.mu.Lock()
				b.hasPin = false
				b.pinOk = true
				b.mu.Unlock()
				return map[string]interface{}{
					"ok":     true,
					"hasPin": false,
					"pinOk":  true,
					"noPin":  true,
				}, nil
			}
			return res, fmt.Errorf("%s", errMsg)
		}
	}

	b.mu.Lock()
	b.hasPin = true
	b.pinOk = true
	b.mu.Unlock()

	if res == nil {
		res = make(map[string]interface{})
	}
	res["ok"] = true
	res["hasPin"] = true
	res["pinOk"] = true
	return res, nil
}

// SetBankPin sets a new 4-digit character PIN on the server.
func (b *BotInstance) SetBankPin(pin string) (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	pw := b.Profile.Password
	b.mu.Unlock()

	cli := b.newGameHTTPClient()
	res, err := cli.SetPin(token, pw, pin, "")
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.hasPin = true
	b.pinOk = true
	b.mu.Unlock()

	if res == nil {
		res = make(map[string]interface{})
	}
	res["ok"] = true
	return res, nil
}

// DepositBankItem moves an item from inventory to account bank.
func (b *BotInstance) DepositBankItem(slotIndex int, count int) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	invList, ok := charData["inv"].([]interface{})
	if !ok || slotIndex < 0 || slotIndex >= len(invList) {
		return nil, fmt.Errorf("invalid inventory slot index %d", slotIndex)
	}

	slotMap, ok := invList[slotIndex].(map[string]interface{})
	if !ok || len(slotMap) == 0 {
		return nil, fmt.Errorf("inventory slot %d is empty", slotIndex)
	}

	if lk, _ := slotMap["lk"].(bool); lk {
		return nil, fmt.Errorf("item is locked")
	}

	avail := toInt(slotMap["n"])
	if avail <= 0 {
		avail = 1
	}
	if count <= 0 || count > avail {
		count = avail
	}

	bankData, err := b.GetBankData()
	if err != nil {
		return nil, err
	}
	bankItems, _ := bankData["items"].([]interface{})
	if bankItems == nil {
		bankItems = make([]interface{}, 42)
		for i := range bankItems {
			bankItems[i] = map[string]interface{}{}
		}
	}

	freeBankIdx := -1
	for i, raw := range bankItems {
		if raw == nil {
			freeBankIdx = i
			break
		}
		if bm, ok := raw.(map[string]interface{}); ok {
			if len(bm) == 0 || bm["id"] == nil || bm["id"] == "" {
				freeBankIdx = i
				break
			}
		}
	}

	if freeBankIdx < 0 {
		return nil, fmt.Errorf("account bank is full (42 slots)")
	}

	// Move item
	cloned := make(map[string]interface{}, len(slotMap))
	for k, v := range slotMap {
		cloned[k] = v
	}
	cloned["n"] = count
	bankItems[freeBankIdx] = cloned

	if count >= avail {
		invList[slotIndex] = map[string]interface{}{}
	} else {
		slotMap["n"] = avail - count
	}

	charData["inv"] = invList
	bankData["items"] = bankItems

	// Persist character and bank files
	b.mu.Lock()
	accKey := b.accountKey
	b.mu.Unlock()
	bankFile := fmt.Sprintf("bank/%s.json", accKey)

	bankBytes, _ := json.Marshal(bankData)
	b.mu.Lock()
	b.saveFiles[bankFile] = string(bankBytes)
	b.mu.Unlock()

	_ = b.persistCharData(charData, charFile, token)
	cli := b.newGameHTTPClient()
	_, _ = cli.SaveFiles(token, map[string]string{
		charFile: b.saveFiles[charFile],
		bankFile: string(bankBytes),
	})

	b.emitLog("action", fmt.Sprintf("Deposited %s x%d into account bank.", slotMap["id"], count), "info")
	bankData["ok"] = true
	bankData["hasPin"] = b.hasPin
	bankData["pinOk"] = true
	return bankData, nil
}

// WithdrawBankItem moves an item from account bank to inventory.
func (b *BotInstance) WithdrawBankItem(bankSlotIndex int, count int) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	invList, _ := charData["inv"].([]interface{})
	if countFreeInventorySlots(invList) < 1 {
		return nil, fmt.Errorf("inventory is full - make room first")
	}

	bankData, err := b.GetBankData()
	if err != nil {
		return nil, err
	}
	bankItems, _ := bankData["items"].([]interface{})
	if bankSlotIndex < 0 || bankSlotIndex >= len(bankItems) {
		return nil, fmt.Errorf("invalid bank slot index %d", bankSlotIndex)
	}

	slotMap, ok := bankItems[bankSlotIndex].(map[string]interface{})
	if !ok || len(slotMap) == 0 {
		return nil, fmt.Errorf("bank slot %d is empty", bankSlotIndex)
	}

	avail := toInt(slotMap["n"])
	if avail <= 0 {
		avail = 1
	}
	if count <= 0 || count > avail {
		count = avail
	}

	cloned := make(map[string]interface{}, len(slotMap))
	for k, v := range slotMap {
		cloned[k] = v
	}
	cloned["n"] = count

	if !insertStructuredItem(invList, cloned) {
		return nil, fmt.Errorf("failed to insert item into inventory")
	}

	if count >= avail {
		bankItems[bankSlotIndex] = map[string]interface{}{}
	} else {
		slotMap["n"] = avail - count
	}

	charData["inv"] = invList
	bankData["items"] = bankItems

	b.mu.Lock()
	accKey := b.accountKey
	b.mu.Unlock()
	bankFile := fmt.Sprintf("bank/%s.json", accKey)

	bankBytes, _ := json.Marshal(bankData)
	b.mu.Lock()
	b.saveFiles[bankFile] = string(bankBytes)
	b.mu.Unlock()

	_ = b.persistCharData(charData, charFile, token)
	cli := b.newGameHTTPClient()
	_, _ = cli.SaveFiles(token, map[string]string{
		charFile: b.saveFiles[charFile],
		bankFile: string(bankBytes),
	})

	b.emitLog("action", fmt.Sprintf("Withdrew %s x%d from account bank.", slotMap["id"], count), "info")
	bankData["ok"] = true
	bankData["hasPin"] = b.hasPin
	bankData["pinOk"] = true
	return bankData, nil
}

// DepositBankPenya moves Penya from character wallet into account bank.
func (b *BotInstance) DepositBankPenya(amount int64) (map[string]interface{}, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("deposit amount must be positive")
	}

	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	curPenya := b.getEffectivePenya(charData)
	if int64(curPenya) < amount {
		return nil, fmt.Errorf("insufficient Penya (have %d, want to deposit %d)", int64(curPenya), amount)
	}

	bankData, err := b.GetBankData()
	if err != nil {
		return nil, err
	}

	curBankPenya := toInt64(bankData["penya"])
	bankData["penya"] = curBankPenya + amount
	b.setEffectivePenya(charData, curPenya-float64(amount))

	b.mu.Lock()
	accKey := b.accountKey
	b.mu.Unlock()

	bankFile := fmt.Sprintf("bank/%s.json", accKey)
	bankBytes, _ := json.Marshal(bankData)
	b.mu.Lock()
	b.saveFiles[bankFile] = string(bankBytes)
	b.mu.Unlock()

	_ = b.persistCharData(charData, charFile, token)
	cli := b.newGameHTTPClient()
	_, _ = cli.SaveFiles(token, map[string]string{
		charFile: b.saveFiles[charFile],
		bankFile: string(bankBytes),
	})

	b.emitLog("action", fmt.Sprintf("Deposited %d Penya into account bank.", amount), "info")
	bankData["ok"] = true
	bankData["hasPin"] = b.hasPin
	bankData["pinOk"] = true
	return bankData, nil
}

// WithdrawBankPenya moves Penya from account bank into character wallet.
func (b *BotInstance) WithdrawBankPenya(amount int64) (map[string]interface{}, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("withdraw amount must be positive")
	}

	bankData, err := b.GetBankData()
	if err != nil {
		return nil, err
	}

	curBankPenya := toInt64(bankData["penya"])
	if curBankPenya < amount {
		return nil, fmt.Errorf("insufficient Bank Penya (have %d, want to withdraw %d)", curBankPenya, amount)
	}

	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	curPenya := b.getEffectivePenya(charData)
	bankData["penya"] = curBankPenya - amount
	b.setEffectivePenya(charData, curPenya+float64(amount))

	b.mu.Lock()
	accKey := b.accountKey
	b.mu.Unlock()

	bankFile := fmt.Sprintf("bank/%s.json", accKey)
	bankBytes, _ := json.Marshal(bankData)
	b.mu.Lock()
	b.saveFiles[bankFile] = string(bankBytes)
	b.mu.Unlock()

	_ = b.persistCharData(charData, charFile, token)
	cli := b.newGameHTTPClient()
	_, _ = cli.SaveFiles(token, map[string]string{
		charFile: b.saveFiles[charFile],
		bankFile: string(bankBytes),
	})

	b.emitLog("action", fmt.Sprintf("Withdrew %d Penya from account bank.", amount), "info")
	bankData["ok"] = true
	bankData["hasPin"] = b.hasPin
	bankData["pinOk"] = true
	return bankData, nil
}

// ============================================================================
// 5. SOCIAL & COUPLE SERVICE
// ============================================================================

// GetCoupleInfo fetches couple relationship status, level, days, and gifts.
func (b *BotInstance) GetCoupleInfo() (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	charSlot := b.getCharSlot()
	return b.executeClaimTransaction(token, map[string]interface{}{
		"k":    "cp_info",
		"char": charSlot,
	})
}

// ClaimCoupleReward claims couple level rewards.
func (b *BotInstance) ClaimCoupleReward(level int) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	charSlot := b.getCharSlot()

	res, err := b.executeClaimTransaction(token, map[string]interface{}{
		"k":     "cp_claim",
		"level": level,
		"char":  charSlot,
	})
	if err != nil {
		return nil, err
	}

	if itemsRaw, ok := res["items"].([]interface{}); ok {
		if invList, ok := charData["inv"].([]interface{}); ok {
			for _, itRaw := range itemsRaw {
				if itArr, ok := itRaw.([]interface{}); ok && len(itArr) >= 2 {
					idStr, _ := itArr[0].(string)
					cnt := int(toFloat32(itArr[1]))
					if idStr != "" {
						addItemToInventory(invList, idStr, cnt)
					}
				}
			}
		}
	}
	_ = b.persistCharData(charData, charFile, token)
	b.flushSave(true)

	b.emitLog("social", fmt.Sprintf("Claimed couple level %d reward!", level), "info")
	return res, nil
}

// ProposeCouple sends a couple proposal to another character.
func (b *BotInstance) ProposeCouple(targetName string) (map[string]interface{}, error) {
	if strings.TrimSpace(targetName) == "" {
		return nil, fmt.Errorf("target name cannot be empty")
	}
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	charSlot := b.getCharSlot()

	res, err := b.executeClaimTransaction(token, map[string]interface{}{
		"k":    "cp_propose",
		"name": strings.TrimSpace(targetName),
		"char": charSlot,
	})
	if err != nil {
		return nil, err
	}
	b.emitLog("social", fmt.Sprintf("Sent couple proposal to %s", targetName), "info")
	return res, nil
}

// BreakCouple breaks off the current couple relationship.
func (b *BotInstance) BreakCouple() (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	charSlot := b.getCharSlot()

	res, err := b.executeClaimTransaction(token, map[string]interface{}{
		"k":    "cp_break",
		"char": charSlot,
	})
	if err != nil {
		return nil, err
	}
	b.emitLog("social", "Couple relationship dissolved.", "info")
	return res, nil
}

// SendPartyInvite sends a party invitation via WebSocket.
func (b *BotInstance) SendPartyInvite(targetName string) error {
	b.mu.Lock()
	if b.Status != "online" || b.ws == nil || !b.ws.IsConnected() {
		b.mu.Unlock()
		return fmt.Errorf("bot is offline")
	}
	b.mu.Unlock()

	pkt := map[string]interface{}{
		"t":    "pinvite",
		"name": strings.TrimSpace(targetName),
	}
	data, _ := json.Marshal(pkt)
	if err := b.ws.Send(data); err != nil {
		return err
	}
	b.emitLog("social", fmt.Sprintf("Invited %s to party.", targetName), "info")
	return nil
}

// AcceptPartyInvite accepts an incoming party invite.
func (b *BotInstance) AcceptPartyInvite(fromID int) error {
	b.mu.Lock()
	if b.Status != "online" || b.ws == nil || !b.ws.IsConnected() {
		b.mu.Unlock()
		return fmt.Errorf("bot is offline")
	}
	b.mu.Unlock()

	pkt := map[string]interface{}{
		"t":    "paccept",
		"from": fromID,
	}
	data, _ := json.Marshal(pkt)
	return b.ws.Send(data)
}

// LeaveParty departs from the current party.
func (b *BotInstance) LeaveParty() error {
	b.mu.Lock()
	if b.Status != "online" || b.ws == nil || !b.ws.IsConnected() {
		b.mu.Unlock()
		return fmt.Errorf("bot is offline")
	}
	b.mu.Unlock()

	pkt := map[string]interface{}{
		"t": "pleave",
	}
	data, _ := json.Marshal(pkt)
	if err := b.ws.Send(data); err != nil {
		return err
	}
	b.emitLog("social", "Left party.", "info")
	return nil
}

// ============================================================================
// 6. PETS & PET FEED MILL SERVICE
// ============================================================================

// HatchPetEgg hatches a pet egg in inventory into a companion pet.
func (b *BotInstance) HatchPetEgg() (map[string]interface{}, error) {
	_, _, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	charSlot := b.getCharSlot()

	res, err := b.executeClaimTransaction(token, map[string]interface{}{
		"k":    "pethatch",
		"char": charSlot,
	})
	if err != nil {
		return nil, err
	}
	b.flushSave(true)
	b.emitLog("action", "Hatched pet egg successfully!", "info")
	return res, nil
}

// MillPetFeed grinds materials from inventory into pet feed.
func (b *BotInstance) MillPetFeed(itemID string, count int) (map[string]interface{}, error) {
	if itemID == "" || count <= 0 {
		return nil, fmt.Errorf("invalid material item or count")
	}
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	charSlot := b.getCharSlot()
	res, err := b.executeClaimTransaction(token, map[string]interface{}{
		"k":    "mill",
		"id":   itemID,
		"n":    count,
		"char": charSlot,
	})
	if err != nil {
		return nil, err
	}

	// Remove milled items and add feed to inventory
	if invList, ok := charData["inv"].([]interface{}); ok {
		removeInventoryItem(invList, itemID, count)
		if feedCount, ok := res["feed"].(float64); ok && feedCount > 0 {
			addItemToInventory(invList, "II_PET_FEED", int(feedCount))
		}
	}
	_ = b.persistCharData(charData, charFile, token)
	b.flushSave(true)

	b.emitLog("action", fmt.Sprintf("Milled %d x %s into Pet Feed.", count, itemID), "info")
	return res, nil
}

// ============================================================================
// HELPERS
// ============================================================================

func removeInventoryItem(invList []interface{}, itemID string, count int) bool {
	if count <= 0 {
		return true
	}
	for i, raw := range invList {
		im, ok := raw.(map[string]interface{})
		if !ok || len(im) == 0 {
			continue
		}
		if id, _ := im["id"].(string); id == itemID {
			n := toInt(im["n"])
			if n <= 0 {
				n = 1
			}
			if n <= count {
				invList[i] = map[string]interface{}{}
				count -= n
			} else {
				im["n"] = n - count
				invList[i] = im
				count = 0
			}
			if count <= 0 {
				return true
			}
		}
	}
	return count <= 0
}

func countFreeInventorySlots(invList []interface{}) int {
	free := 0
	for _, raw := range invList {
		if raw == nil {
			free++
			continue
		}
		if m, ok := raw.(map[string]interface{}); ok {
			if len(m) == 0 || m["id"] == nil || m["id"] == "" {
				free++
			}
		} else {
			free++
		}
	}
	return free
}

func insertStructuredItem(invList []interface{}, it map[string]interface{}) bool {
	if it == nil || len(it) == 0 {
		return false
	}
	clean := make(map[string]interface{}, len(it))
	for k, v := range it {
		clean[k] = v
	}
	n := 1
	if nVal := toInt(clean["n"]); nVal > 0 {
		n = nVal
	}
	clean["n"] = n

	for i, raw := range invList {
		if raw == nil {
			invList[i] = clean
			return true
		}
		if m, ok := raw.(map[string]interface{}); ok {
			if len(m) == 0 || m["id"] == nil || m["id"] == "" {
				invList[i] = clean
				return true
			}
		}
	}
	if len(invList) < 240 {
		invList = append(invList, clean)
		return true
	}
	return false
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	}
	return 0
}

// ============================================================================
// 7. DAILY REWARDS STATUS
// ============================================================================

// GetDailyRewardStatus checks if today's login gift has already been claimed.
func (b *BotInstance) GetDailyRewardStatus() (map[string]interface{}, error) {
	_, charFile, _, err := b.loadCharData()
	if err != nil {
		return map[string]interface{}{
			"ok":           true,
			"day":          1,
			"claimedToday": false,
		}, nil
	}

	todayStr := time.Now().Format("2006-01-02")
	rewardsFile := strings.TrimSuffix(charFile, ".json") + "_rewards.json"
	b.mu.Lock()
	rawRewards, hasRewards := b.saveFiles[rewardsFile]
	b.mu.Unlock()

	dayNum := 1
	claimedToday := false
	lastClaimDate := ""

	if hasRewards && rawRewards != "" {
		var rState map[string]interface{}
		if json.Unmarshal([]byte(rawRewards), &rState) == nil {
			if d, ok := rState["day"].(float64); ok {
				dayNum = int(d)
			}
			if last, ok := rState["last"].(string); ok {
				lastClaimDate = last
				if last == todayStr {
					claimedToday = true
				}
			}
		}
	}

	return map[string]interface{}{
		"ok":            true,
		"day":           dayNum,
		"claimedToday":  claimedToday,
		"lastClaimDate": lastClaimDate,
	}, nil
}

// ============================================================================
// 8. ACHIEVEMENTS & TITLES
// ============================================================================

type AchievementDef struct {
	ID     string   `json:"id"`
	Tab    string   `json:"tab"`
	Name   string   `json:"name"`
	Desc   string   `json:"desc"`
	Steps  []int    `json:"steps"`
	Titles []string `json:"titles"`
	Icon   string   `json:"icon"`
}

var achievementCatalog = []AchievementDef{
	{
		ID:     "level",
		Tab:    "general",
		Name:   "Road to Legend",
		Desc:   "Reach level %d.",
		Steps:  []int{15, 30, 60, 90, 120},
		Titles: []string{"Rookie", "Adventurer", "Veteran", "Hero", "Legend of Madrigal"},
		Icon:   "emoji_events",
	},
	{
		ID:     "kills",
		Tab:    "general",
		Name:   "Monster Hunter",
		Desc:   "Defeat %d monsters.",
		Steps:  []int{100, 1000, 10000, 50000, 200000},
		Titles: []string{"Hunter", "Slayer", "Executioner", "Annihilator", "Godslayer"},
		Icon:   "swords",
	},
	{
		ID:     "bosses",
		Tab:    "general",
		Name:   "Giant Killer",
		Desc:   "Defeat %d giants and bosses.",
		Steps:  []int{1, 10, 50, 200, 1000},
		Titles: []string{"Giant Killer", "Titan Breaker", "Boss Hunter", "Colossus Bane", "Worldbreaker"},
		Icon:   "military_tech",
	},
	{
		ID:     "penya",
		Tab:    "general",
		Name:   "Wealthy",
		Desc:   "Hold %d Penya at once.",
		Steps:  []int{100000, 1000000, 10000000, 100000000, 1000000000},
		Titles: []string{"Penny Pincher", "Merchant", "Tycoon", "Magnate", "Penya Baron"},
		Icon:   "receipt_long",
	},
	{
		ID:     "play",
		Tab:    "general",
		Name:   "Dedicated",
		Desc:   "Play for %d hours.",
		Steps:  []int{1, 10, 50, 200, 1000},
		Titles: []string{"Newcomer", "Regular", "Dedicated", "No-Lifer", "Eternal"},
		Icon:   "history",
	},
	{
		ID:     "quests",
		Tab:    "adventure",
		Name:   "Quest Seeker",
		Desc:   "Finish %d quests.",
		Steps:  []int{10, 50, 150, 300, 500},
		Titles: []string{"Errand Runner", "Problem Solver", "Quest Master", "Living Legend", "Saviour of Madrigal"},
		Icon:   "assignment",
	},
	{
		ID:     "upg",
		Tab:    "adventure",
		Name:   "Blacksmith",
		Desc:   "Upgrade gear successfully %d times.",
		Steps:  []int{10, 50, 200, 500, 1000},
		Titles: []string{"Apprentice Smith", "Smith", "Master Smith", "Forgemaster", "Divine Smith"},
		Icon:   "auto_fix_high",
	},
	{
		ID:     "deaths",
		Tab:    "adventure",
		Name:   "Clumsy",
		Desc:   "Fall in battle %d times.",
		Steps:  []int{1, 25, 100, 300, 1000},
		Titles: []string{"Clumsy", "Reckless", "Daredevil", "Undying", "Ghost"},
		Icon:   "warning",
	},
}

// GetAchievements returns player's progress across all achievements and titles.
func (b *BotInstance) GetAchievements() (map[string]interface{}, error) {
	charData, charFile, _, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	achFile := strings.TrimSuffix(charFile, ".json") + "_ach.json"
	b.mu.Lock()
	rawAch, hasAch := b.saveFiles[achFile]
	b.mu.Unlock()

	achData := map[string]interface{}{
		"k":     map[string]interface{}{},
		"got":   map[string]interface{}{},
		"title": "",
		"sp":    []interface{}{},
	}
	if hasAch && rawAch != "" {
		_ = json.Unmarshal([]byte(rawAch), &achData)
	}

	currentTitle, _ := achData["title"].(string)

	// Determine stat values for achievements
	b.mu.Lock()
	curLevel := int(b.Stats.Level)
	playHours := int(b.Stats.PlayedSeconds / 3600)
	deaths := int(b.Stats.Deaths)
	b.mu.Unlock()

	penya := int(b.getEffectivePenya(charData))

	killCount := 0
	if kMap, ok := achData["k"].(map[string]interface{}); ok {
		for _, v := range kMap {
			killCount += int(toFloat32(v))
		}
	}

	gotMap, _ := achData["got"].(map[string]interface{})
	if gotMap == nil {
		gotMap = make(map[string]interface{})
	}

	var achList []map[string]interface{}
	var unlockedTitles []map[string]interface{}
	completedLevels := 0
	totalPossible := 0

	for _, a := range achievementCatalog {
		val := 0
		switch a.ID {
		case "level":
			val = curLevel
		case "kills":
			val = killCount
		case "bosses":
			val = int(toFloat32(gotMap["bosses_val"]))
		case "penya":
			val = penya
		case "play":
			val = playHours
		case "quests":
			val = int(toFloat32(gotMap["quests_val"]))
		case "upg":
			val = int(toFloat32(gotMap["upg_val"]))
		case "deaths":
			val = deaths
		}

		achievedLv := 0
		for _, step := range a.Steps {
			if val >= step {
				achievedLv++
			}
		}

		completedLevels += achievedLv
		totalPossible += len(a.Steps)

		nextTarget := a.Steps[len(a.Steps)-1]
		if achievedLv < len(a.Steps) {
			nextTarget = a.Steps[achievedLv]
		}

		// Collect unlocked titles
		for i := 0; i < achievedLv && i < len(a.Titles); i++ {
			t := a.Titles[i]
			if t != "" {
				unlockedTitles = append(unlockedTitles, map[string]interface{}{
					"title":      t,
					"source":     a.Name,
					"tier":       i + 1,
					"isEquipped": (t == currentTitle),
				})
			}
		}

		achList = append(achList, map[string]interface{}{
			"id":        a.ID,
			"tab":       a.Tab,
			"name":      a.Name,
			"desc":      fmt.Sprintf(a.Desc, nextTarget),
			"current":   val,
			"target":    nextTarget,
			"level":     achievedLv,
			"maxLevel":  len(a.Steps),
			"steps":     a.Steps,
			"titles":    a.Titles,
			"icon":      a.Icon,
			"completed": achievedLv >= len(a.Steps),
		})
	}

	// Add special titles if any
	if spList, ok := achData["sp"].([]interface{}); ok {
		for _, spRaw := range spList {
			if spTitle, ok := spRaw.(string); ok && spTitle != "" {
				unlockedTitles = append(unlockedTitles, map[string]interface{}{
					"title":      spTitle,
					"source":     "Special Event",
					"tier":       1,
					"isEquipped": (spTitle == currentTitle),
				})
			}
		}
	}

	return map[string]interface{}{
		"ok":              true,
		"achievements":    achList,
		"titles":          unlockedTitles,
		"currentTitle":    currentTitle,
		"completedLevels": completedLevels,
		"totalLevels":     totalPossible,
	}, nil
}

// SetTitle equips or unequips a character title.
func (b *BotInstance) SetTitle(title string) (map[string]interface{}, error) {
	_, charFile, token, err := b.loadCharData()
	if err != nil {
		return map[string]interface{}{"ok": false, "error": err.Error()}, nil
	}

	achFile := strings.TrimSuffix(charFile, ".json") + "_ach.json"
	b.mu.Lock()
	rawAch, hasAch := b.saveFiles[achFile]
	b.mu.Unlock()

	achData := map[string]interface{}{
		"title": title,
	}
	if hasAch && rawAch != "" {
		_ = json.Unmarshal([]byte(rawAch), &achData)
		achData["title"] = title
	}

	if aBytes, mErr := json.Marshal(achData); mErr == nil {
		b.mu.Lock()
		b.saveFiles[achFile] = string(aBytes)
		b.mu.Unlock()

		if token != "" {
			cli := b.newGameHTTPClient()
			_, _ = cli.SaveFiles(token, map[string]string{achFile: string(aBytes)})
		}
	}

	b.emitLog("system", fmt.Sprintf("Equipped title: \"%s\"", title), "info")
	return map[string]interface{}{
		"ok":    true,
		"title": title,
	}, nil
}

// ============================================================================
// 9. MONSTER ALBUM
// ============================================================================

type MonsterAlbumEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Level   int    `json:"level"`
	Zone    string `json:"zone"`
	PieceID string `json:"piece_id"`
}

var monsterAlbumList = []MonsterAlbumEntry{
	{"MI_AIBATT4", "Giant Aibatt", 5, "Flarine", "II_MONSTERALBUM_AIBATT_PIECE"},
	{"MI_MUSHPANG4", "Giant Mushpang", 9, "Flarine", "II_MONSTERALBUM_MUSHPANG_PIECE"},
	{"MI_BURUDENG4", "Giant Burudeng", 13, "Flarine", "II_MONSTERALBUM_BURUDENG_PIECE"},
	{"MI_PUKEPUKE4", "Giant Pukepuke", 17, "Flarine", "II_MONSTERALBUM_PUKEPUKE_PIECE"},
	{"MI_PEAKYTURTLE4", "Giant Peakyturtle", 21, "Flarine", "II_MONSTERALBUM_PEAKYTURTLE_PIECE"},
	{"MI_DEMIAN4", "Giant Demian", 25, "Flarine", "II_MONSTERALBUM_DEMIAN_PIECE"},
	{"MI_DORIDOMA4", "Giant Doridoma", 29, "Flarine", "II_MONSTERALBUM_DORIDOMA_PIECE"},
	{"MI_LAWOLF4", "Giant Lawolf", 33, "Saint Morning", "II_MONSTERALBUM_LAWOLF_PIECE"},
	{"MI_FEFERN4", "Giant Fefern", 37, "Saint Morning", "II_MONSTERALBUM_FEFERN_PIECE"},
	{"MI_NYANGNYANG4", "Giant Nyangnyang", 41, "Saint Morning", "II_MONSTERALBUM_NYANGNYANG_PIECE"},
	{"MI_BANG4", "Giant Bang", 45, "Saint Morning", "II_MONSTERALBUM_BANG_PIECE"},
	{"MI_WAGSAAC4", "Giant Wagsaac", 49, "Saint Morning", "II_MONSTERALBUM_WAGSAAC_PIECE"},
	{"MI_MIA4", "Giant Mia", 53, "Saint Morning", "II_MONSTERALBUM_MIA_PIECE"},
	{"MI_MRPUMPKIN4", "Giant Mr. Pumpkin", 57, "Saint Morning", "II_MONSTERALBUM_MRPUMPKIN_PIECE"},
	{"MI_REDMANTIS4", "Giant Redmantis", 61, "Saint Morning", "II_MONSTERALBUM_REDMANTIS_PIECE"},
	{"MI_JACKTHEHAMMER4", "Giant Jack The Hammer", 65, "Saint Morning", "II_MONSTERALBUM_JACKTHEHAMMER_PIECE"},
	{"MI_GIGGLEBOX4", "Giant Gigglebox", 69, "Saint Morning", "II_MONSTERALBUM_GIGGLEBOX_PIECE"},
	{"MI_ROCKMUSCLE4", "Giant Rockmuscle", 73, "Saint Morning", "II_MONSTERALBUM_ROCKMUSCLE_PIECE"},
	{"MI_HOBO4", "Giant Hobo", 77, "Darkon 1", "II_MONSTERALBUM_HOBO_PIECE"},
	{"MI_DUMBBULL4", "Giant Dumbbull", 81, "Darkon 1", "II_MONSTERALBUM_DUMBBULL_PIECE"},
	{"MI_TOTEMIA4", "Giant Totemia", 85, "Darkon 1", "II_MONSTERALBUM_TOTEMIA_PIECE"},
	{"MI_CARDPUPPET4", "Giant Cardpuppet", 89, "Darkon 1", "II_MONSTERALBUM_CARDPUPPET_PIECE"},
	{"MI_TOMBSTONEBEARER4", "Giant Tombstone Bearer", 93, "Darkon 1", "II_MONSTERALBUM_TOMBSTONEBEARER_PIECE"},
	{"MI_BASQUE4", "Giant Basque", 97, "Darkon 1", "II_MONSTERALBUM_BASQUE_PIECE"},
	{"MI_PRANKSTER4", "Giant Prankster", 101, "Darkon 2", "II_MONSTERALBUM_PRANKSTER_PIECE"},
	{"MI_WHEELEM4", "Giant Wheelem", 105, "Darkon 2", "II_MONSTERALBUM_WHEELEM_PIECE"},
	{"MI_LEYENA4", "Giant Leyena", 109, "Darkon 2", "II_MONSTERALBUM_LEYENA_PIECE"},
	{"MI_STEAMWALKER4", "Giant Steamwalker", 113, "Darkon 2", "II_MONSTERALBUM_STEAMWALKER_PIECE"},
	{"MI_STEELKNIGHT4", "Giant Steelknight", 117, "Darkon 2", "II_MONSTERALBUM_STEELKNIGHT_PIECE"},
	{"MI_DRILLER4", "Giant Driller", 121, "Darkon 3", "II_MONSTERALBUM_DRILLER_PIECE"},
	{"MI_VOLT4", "Giant Volt", 125, "Darkon 3", "II_MONSTERALBUM_VOLT_PIECE"},
	{"MI_ELDERGUARD4", "Giant Elderguard", 129, "Darkon 3", "II_MONSTERALBUM_ELDERGUARD_PIECE"},
	{"MI_GARBAGEPIDER4", "Giant Garbagepider", 133, "Darkon 3", "II_MONSTERALBUM_GARBAGEPIDER_PIECE"},
	{"MI_CRANEMACHINERY4", "Giant Crane Machinery", 137, "Darkon 3", "II_MONSTERALBUM_CRANEMACHINERY_PIECE"},
	{"MI_SYLIACA4", "Giant Syliaca", 141, "Darkon 3", "II_MONSTERALBUM_SYLIACA_PIECE"},
	{"MI_GREEMONG4", "Giant Greemong", 145, "Darkon 3", "II_MONSTERALBUM_GREEMONG_PIECE"},
	{"MI_BOO4", "Giant Boo", 149, "Darkon 3", "II_MONSTERALBUM_BOO_PIECE"},
	{"MI_MUSHPOIE4", "Giant Mushpoie", 153, "Darkon 3", "II_MONSTERALBUM_MUSHPOIE_PIECE"},
	{"MI_IREN4", "Giant Iren", 157, "Darkon 3", "II_MONSTERALBUM_IREN_PIECE"},
	{"MI_WATANGKA4", "Giant Watangka", 161, "Darkon 3", "II_MONSTERALBUM_WATANGKA_PIECE"},
	{"MI_ANTIQUERY4", "Giant Antiquery", 165, "Darkon 3", "II_MONSTERALBUM_ANTIQUERY_PIECE"},
	{"MI_GLAPHAN4", "Giant Glaphan", 170, "Darkon 3", "II_MONSTERALBUM_GLAPHAN_PIECE"},
	{"MI_LORDBANG", "Lord Bang", 50, "Dungeon", "II_MONSTERALBUM_LORDBANG_PIECE"},
	{"MI_KIMERADON5", "Kimeradon", 115, "Dungeon", "II_MONSTERALBUM_KIMERADON_PIECE"},
	{"MI_BEARNUCKY5", "Bearnucky", 120, "Dungeon", "II_MONSTERALBUM_BEARNUCKY_PIECE"},
}

// GetMonsterAlbum fetches completed cards, pieces needed, and stat bonuses.
func (b *BotInstance) GetMonsterAlbum() (map[string]interface{}, error) {
	charData, _, _, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	albumMap, _ := charData["album"].(map[string]interface{})
	if albumMap == nil {
		albumMap = make(map[string]interface{})
	}

	// Count card pieces in bot inventory
	invList, _ := charData["inv"].([]interface{})
	bagPieces := make(map[string]int)
	if invList != nil {
		for _, rawSlot := range invList {
			if slot, ok := rawSlot.(map[string]interface{}); ok {
				itemID, _ := slot["id"].(string)
				if strings.HasPrefix(itemID, "II_MONSTERALBUM_") {
					n := toInt(slot["n"])
					if n <= 0 {
						n = 1
					}
					bagPieces[itemID] += n
				}
			}
		}
	}

	completedCount := 0
	var cards []map[string]interface{}

	for _, mob := range monsterAlbumList {
		pieces := toInt(albumMap[mob.ID])
		isCompleted := pieces >= 5
		if isCompleted {
			completedCount++
			pieces = 5
		}

		inBag := bagPieces[mob.PieceID]

		cards = append(cards, map[string]interface{}{
			"id":        mob.ID,
			"name":      mob.Name,
			"level":     mob.Level,
			"zone":      mob.Zone,
			"piece_id":  mob.PieceID,
			"pieces":    pieces,
			"needed":    5,
			"completed": isCompleted,
			"in_bag":    inBag,
		})
	}

	// Calculate album bonus based on completed cards count
	var bonuses []string
	if completedCount >= 1 {
		bonuses = append(bonuses, "ATK Power +20")
	}
	if completedCount >= 5 {
		bonuses = append(bonuses, "STR +10, STA +10")
	}
	if completedCount >= 10 {
		bonuses = append(bonuses, "Max HP +15%")
	}
	if completedCount >= 20 {
		bonuses = append(bonuses, "DEX +15, Speed +10%")
	}
	if completedCount >= 30 {
		bonuses = append(bonuses, "All Stats +20")
	}
	if completedCount >= 40 {
		bonuses = append(bonuses, "ATK +5%, Max HP +5%")
	}
	if completedCount >= 45 {
		bonuses = append(bonuses, "Monster DMG +10%")
	}

	return map[string]interface{}{
		"ok":             true,
		"cards":          cards,
		"completedCount": completedCount,
		"totalCards":     len(monsterAlbumList),
		"needed":         5,
		"bonuses":        bonuses,
	}, nil
}

// UseMonsterAlbumPiece registers a card piece from inventory into the monster album.
func (b *BotInstance) UseMonsterAlbumPiece(mobID string) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return map[string]interface{}{"ok": false, "error": err.Error()}, nil
	}

	// Find the matching monster entry
	var targetMob *MonsterAlbumEntry
	for i := range monsterAlbumList {
		if monsterAlbumList[i].ID == mobID {
			targetMob = &monsterAlbumList[i]
			break
		}
	}
	if targetMob == nil {
		return map[string]interface{}{"ok": false, "error": "Unknown monster card"}, nil
	}

	albumMap, ok := charData["album"].(map[string]interface{})
	if !ok || albumMap == nil {
		albumMap = make(map[string]interface{})
		charData["album"] = albumMap
	}

	curPieces := toInt(albumMap[mobID])
	if curPieces >= 5 {
		return map[string]interface{}{"ok": false, "error": "This monster card is already fully completed!"}, nil
	}

	// Consume 1 piece item from inventory
	invList, _ := charData["inv"].([]interface{})
	consumed := false
	if invList != nil {
		for i, rawSlot := range invList {
			if slot, ok := rawSlot.(map[string]interface{}); ok {
				itemID, _ := slot["id"].(string)
				if itemID == targetMob.PieceID {
					n := toInt(slot["n"])
					if n > 1 {
						slot["n"] = n - 1
					} else {
						invList[i] = map[string]interface{}{}
					}
					consumed = true
					break
				}
			}
		}
	}

	if !consumed {
		return map[string]interface{}{
			"ok":    false,
			"error": fmt.Sprintf("No %s pieces found in your inventory to register.", targetMob.Name),
		}, nil
	}

	newPieces := curPieces + 1
	albumMap[mobID] = newPieces
	charData["inv"] = invList

	_ = b.persistCharData(charData, charFile, token)

	isComplete := newPieces >= 5
	msg := fmt.Sprintf("Added card piece for %s (%d/5).", targetMob.Name, newPieces)
	if isComplete {
		msg = fmt.Sprintf("🎉 Congratulations! %s card completed in Monster Album!", targetMob.Name)
	}
	b.emitLog("system", msg, "info")

	return map[string]interface{}{
		"ok":        true,
		"pieces":    newPieces,
		"completed": isComplete,
		"message":   msg,
	}, nil
}
