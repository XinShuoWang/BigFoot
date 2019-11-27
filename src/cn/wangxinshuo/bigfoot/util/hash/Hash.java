package cn.wangxinshuo.bigfoot.util.hash;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;

public class Hash {
    public static String md5(String src) {
        try {
            // 加密对象，指定加密方式
            MessageDigest md5 = MessageDigest.getInstance("md5");
            // 准备要加密的数据
            byte[] b = src.getBytes();
            // 加密
            byte[] digest = md5.digest(b);
            // 十六进制的字符
            char[] chars = new char[]{'0', '1', '2', '3', '4', '5',
                    '6', '7', '8', '9', 'A', 'B', 'C', 'D', 'E', 'F'};
            StringBuilder sb = new StringBuilder();
            // 处理成十六进制的字符串(通常)
            for (byte bb : digest) {
                sb.append(chars[(bb >> 4) & 15]);
                sb.append(chars[bb & 15]);
            }
            // 打印加密后的字符串
            return sb.toString();
        } catch (NoSuchAlgorithmException e) {
            e.printStackTrace();
        }
        return null;
    }

    private static String byte2Hex(byte[] bytes) {
        StringBuilder stringBuffer = new StringBuilder();
        String temp = null;
        for (byte aByte : bytes) {
            temp = Integer.toHexString(aByte & 0xFF);
            if (temp.length() == 1) {
                //1得到一位的进行补0操作
                stringBuffer.append("0");
            }
            stringBuffer.append(temp);
        }
        return stringBuffer.toString();
    }

    public static String sha(String src, int bits) {
        MessageDigest messageDigest;
        String encodeStr = "";
        try {
            switch (bits){
                case 256:
                    messageDigest = MessageDigest.getInstance("SHA-256");
                case 512:
                    messageDigest = MessageDigest.getInstance("SHA-512");
                default:
                    messageDigest = MessageDigest.getInstance("SHA-256");
            }
            messageDigest.update(src.getBytes(StandardCharsets.UTF_8));
            encodeStr = byte2Hex(messageDigest.digest());
        } catch (NoSuchAlgorithmException e) {
            e.printStackTrace();
        }
        return encodeStr;
    }
}
