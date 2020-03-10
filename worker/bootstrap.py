from shapely.geometry import Point
import sys
import json

from utils.io import oprint, eprint
from utils.path import file_exists
from utils.file import json_from_file

from registry.check_geofile import REGISTRY


def main():
    exist = file_exists(sys.argv[1])

    if exist:
        geojson = json_from_file(sys.argv[1])
        if geojson :
            # with open(geojson, 'r') as f:
            #     try:
            #         checks = sys.argv[2].split()
            #         for check in checks:
            #             REGISTRY[check](sys.argv[1])
            #     except Exception as e:
            #         eprint("cannot convert args to json object :", str(e), sys.argv[2])
            print("OPENED")
        else :
            raise FileNotFoundError("cannot convert file to geojson : {}".format(sys.argv[1]))
    else :
        raise FileNotFoundError("cannot find input file : {}".format(sys.argv[1]))

if __name__ == "__main__":
    main()
