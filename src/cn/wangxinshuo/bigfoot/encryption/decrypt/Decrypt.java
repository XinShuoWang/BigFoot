package cn.wangxinshuo.bigfoot.encryption.decrypt;

import cn.wangxinshuo.bigfoot.conf.Configuration;

public class Decrypt {
    public static byte[] decrypt(byte[] data, int len, Configuration configuration){
        byte[] bytes = new byte[len];
//        for (int i = 0; i < len; i++) {
//            bytes[i] = (byte) ((byte) (configuration.getPassword()) ^ data[i]);
//        }
        System.arraycopy(data,0,bytes,0,len);
        return bytes;
    }
}