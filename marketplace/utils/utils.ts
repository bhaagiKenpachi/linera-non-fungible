// export const GetIpfsUrlFromPinata = (pinataUrl: string) => {
//     const IPFSUrls = pinataUrl.split("/");
//     const lastIndex = IPFSUrls.length;
//     const IPFSUrl = "https://ipfs.io/ipfs/"+IPFSUrls[lastIndex-1];
//     return IPFSUrl;
// };

export const GetIpfsUrlFromPinata = (pinataUrl: string) => {
    if (!pinataUrl || !pinataUrl.includes("ipfs://")) {
      // console.warn("Invalid Pinata URL", pinataUrl)
      return pinataUrl
    }
  
    const IPFSUrl = pinataUrl.replace("ipfs://", "https://ipfs.io/ipfs/")
    return IPFSUrl
  }
  
  