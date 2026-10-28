-- down 不恢复 passkeys/device_links(退役表;需回滚时从 0011 前的备份重建)
DROP TABLE IF EXISTS auth_identities;
