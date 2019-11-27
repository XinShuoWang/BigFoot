package cn.wangxinshuo.bigfoot.util.encryption.operation;

public class Byte {
    public static byte[] byteEncrypt(byte[] data){
        return byteOperation(data);
    }

    public static byte[] byteDecrypt(byte[] data){
        return byteOperation(data);
    }

    private static byte[] byteOperation(byte[] data){
        byte[] bytes = new byte[data.length];
        for (int i = 0; i < data.length; i++) {
            if(data[i] == -128){
                bytes[i] = data[i];
            }else{
                bytes[i] = (byte) -data[i];
            }
        }
        return bytes;
    }
}
