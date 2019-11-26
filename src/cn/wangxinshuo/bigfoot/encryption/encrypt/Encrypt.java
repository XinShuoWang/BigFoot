package cn.wangxinshuo.bigfoot.encryption.encrypt;

import cn.wangxinshuo.bigfoot.conf.Configuration;

public class Encrypt {
    public static byte[] encrypt(byte[] data, int len, Configuration configuration){
        byte[] bytes = new byte[len];
//        for (int i = 0; i < len; i++) {
//            bytes[i] = (byte) ((byte) (configuration.getPassword()) ^ data[i]);
//        }
        System.arraycopy(data,0,bytes,0,len);
        return bytes;
    }
}
