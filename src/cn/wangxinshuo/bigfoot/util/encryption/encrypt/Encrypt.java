package cn.wangxinshuo.bigfoot.util.encryption.encrypt;

import cn.wangxinshuo.bigfoot.conf.Configuration;

public class Encrypt {
    public static byte[] encrypt(byte[] data, int len, Configuration configuration){
        byte[] bytes = new byte[len];
        for (int i = 0; i < len; i++) {
            if(data[i] == -128){
                bytes[i] = data[i];
            }else{
                bytes[i] = (byte) -data[i];
            }
        }
        //System.arraycopy(data,0,bytes,0,len);
        return bytes;
    }
}